# mrpc - 极简 RPC

一个基于 TCP + 反射的极简 Go RPC 框架，使用 msgpack 序列化，专注于**单向非流式调用**。

## 特性

- **零依赖运行时**：仅依赖 msgpack 用于消息序列化
- **反射驱动**：通过 `Register` 自动发现满足 `func(req T, reply *U) error` 签名的方法
- **单向非流式**：每次连接处理一个请求→一个响应，简单可靠
- **长连接复用**：客户端在多次 Call 之间复用 TCP 连接
- **分级缓冲池**：`TieredPool` 减少 GC 压力（可选）
- **内置健康检查**：开箱即用的服务端心跳检测

## 安装

```bash
go get github.com/23jdd/mrpc
```

## 快速开始

### 1. 定义服务

```go
type Calculator struct{}

type MultiplyReq struct {
    A int
    B int
}

type MultiplyReply struct {
    Product int
}

// 签名必须为 func(req T, reply *U) error
func (c *Calculator) Multiply(req MultiplyReq, reply *MultiplyReply) error {
    reply.Product = req.A * req.B
    return nil
}
```

### 2. 启动服务端

```go
lis, _ := net.Listen("tcp", "127.0.0.1:8080")
server := mrpc.NewServer(lis)

// 注册服务（名字用于客户端路由）
server.Register("Calculator", &Calculator{})

// 启动（阻塞）
server.Run()
```

### 3. 客户端调用

```go
client := mrpc.NewClient("127.0.0.1", 8080)
defer client.Close()

req := &MultiplyReq{A: 7, B: 8}
var reply MultiplyReply
err := client.Call("Calculator.Multiply", req, &reply)
// reply.Product == 56
```

## API 参考

### Server

```go
func NewServer(lis net.Listener) *Server
```

创建 RPC 服务端。`lis` 必须非 nil。

```go
func (s *Server) Register(name string, target any) error
```

注册一个 RPC 服务实现。

**参数：**
- `name`: 服务名，客户端使用 `"name.MethodName"` 格式调用
- `target`: 服务实现，**必须是指向 struct 的指针**（如 `&Calculator{}`）

**注册规则：**
- 自动扫描 `target` 的所有导出方法
- 只注册满足 `func(req T, reply *U) error` 的方法
- 同名服务重复注册会覆盖之前的方法
- 返回 `nil` 表示成功，否则返回错误描述

**边界条件：**
| 输入 | 行为 |
|------|------|
| `target` 为值类型（非指针） | 返回错误 `"target must be a pointer to struct"` |
| `target` 为 `*int` 等非结构体指针 | 返回错误 `"target must point to a struct"` |
| `name` 为空字符串 | 合法，方法注册为 `".MethodName"`（不推荐） |
| 结构体无符合签名的方法 | 返回 `nil`（不报错），但不注册任何方法 |

```go
func (s *Server) Run()
```

启动服务端主循环，阻塞直到 `lis.Accept()` 返回不可恢复错误。


### Client

```go
func NewClient(address string, port int) *Client
```

创建一个 RPC 客户端。

**参数：**
- `address`: 服务端地址，支持 `"127.0.0.1"`、`"localhost"`、`"::1"` 等
- `port`: 服务端端口号

```go
func (c *Client) Dial() error
```

建立到服务端的 TCP 连接。若已有连接则先关闭旧连接。

```go
func (c *Client) Call(method string, argv any, reply any) error
```

执行一次单向非流式 RPC 调用。

**流程：**
1. 若未连接则自动 `Dial()`
2. `Encode(argv)` → 发送请求帧
3. 接收响应帧 → `Decode(reply)`

**参数：**
- `method`: `"ServiceName.MethodName"`
- `argv`: 请求参数指针（msgpack 需要指针才能编码）
- `reply`: 响应参数指针（结果解码到此），可为 `nil`（跳过解码）

**返回值：**
- `nil`: 调用成功
- `ErrShutdown`: 客户端已关闭
- 其他错误：网络错误、编解码错误、服务端返回的错误

**边界条件：**
| 场景 | 行为 |
|------|------|
| 未调用 `Dial` 直接 `Call` | 自动拨号 |
| `argv` 为 `nil` | msgpack 编码为 `nil`，解码端需处理 |
| `reply` 为 `nil` | 跳过响应解码 |
| 服务端返回 error | `Call` 返回包含错误消息的 `error` |
| 网络断开 | 返回网络错误，下次 `Call` 自动重连 |
| 并发调用 `Call` | **不安全**，需要外部加锁 |
| 调用已 `Close` 的客户端 | 返回 `ErrShutdown` |

```go
func (c *Client) Close() error
```

关闭连接并标记客户端为 shut down 状态。幂等，可多次调用。

### 健康检查

```go
func RegisterHealth() *healthService
```

返回一个实现了 `Health.Check` 方法的服务实例，注册后客户端可调用：

```go
server.Register("Health", mrpc.RegisterHealth())

// 客户端：
var reply mrpc.HealthReply
client.Call("Health.Check", &mrpc.HealthRequest{}, &reply)
// reply.Ok == true 表示服务正常
```

```go
func NewHealthChecker(interval, timeout time.Duration) *HealthChecker
func (hc *HealthChecker) Start(client *Client, maxFailures int, onFailure func(error))
func (hc *HealthChecker) Stop()
```

周期性健康检查器。`Start` 在后台 goroutine 中定期调用 `Health.Check`，连续 `maxFailures` 次失败后调用 `onFailure`。

### 分级缓冲池（可选）

```go
func NewTieredPool(capacities ...int) *TieredPool
func (tp *TieredPool) Get(size int) []byte
func (tp *TieredPool) Put(buf []byte)
```

按容量分桶的缓冲池，减少 GC 压力。当前未直接集成到 RPC 调用中（可手动使用）。

### 协议

`Request` 和 `Response` 的低级 API：

```go
// 帧协议
func WriteFrame(w io.Writer, payload []byte) error
func ReadFrame(r io.Reader) ([]byte, error)

// 请求
func NewRequest(method string, seq uint64, argv []byte) *Request
func SendRequest(w io.Writer, req *Request) error
func ReceiveRequest(r io.Reader) (*Request, error)

// 响应
func NewResponse(seq uint64, reply []byte, err string) *Response
func SendResponse(w io.Writer, resp *Response) error
func ReceiveResponse(r io.Reader) (*Response, error)
```

**帧格式：**
```
[4 字节 BigEndian 总长度] [payload...]
```
- 最大 payload：`10MB` (`MaxPayloadSize`)
- 超限时返回 `ErrMaxPayload`

### 编解码器

```go
type Codec interface {
    Encode(v any) ([]byte, error)
    Decode(data []byte, v any) error
}

func NewMsgCodec() *MsgCodec
```

默认使用 msgpack。可通过实现 `Codec` 接口替换为 JSON、Protobuf 等。

## 线程安全

| 组件 | 安全性 |
|------|--------|
| `Server.Register` | 非线程安全，需在 `Run()` 前完成注册 |
| `Server.Run` | 每个连接独立 goroutine，并发安全 |
| `Server.Call` | 非线程安全，仅供单 goroutine 调用 |
| `Client.Call` | **非线程安全**，同一连接上的并发调用需外部加锁 |
| `Client.Close` | 线程安全（可通过 `sync.Once` 包装） |
| `TieredPool` | `Get`/`Put` 线程安全（通过 `sync.Pool`） |

## 限制

- **单向非流式**：不支持服务端推送、流式传输
- **短连接处理**：服务端每个连接只处理一个请求后关闭
- **无超时控制**：`Call` 无内置超时（可通过 `context` 在外部控制）
- **无 TLS**：纯 TCP 通信，生产环境建议搭配 sidecar 或 VPN

## 运行测试

```bash
go test -v ./...
```

## 项目结构

```
mrpc/
├── client.go          # RPC 客户端
├── server.go          # RPC 服务端（反射注册 + TCP 监听）
├── protocol.go        # 二进制线协议（帧 + Request/Response）
├── codec.go           # Codec 接口 + msgpack 实现
├── pool.go            # 分级缓冲池（Tiered Pool）
├── health.go          # 健康检查（服务端 + 客户端检查器）
├── cmd/main.go        # 可运行的示例程序
├── *_test.go          # 单元测试 & 集成测试 & 示例测试
```

## License

MIT
