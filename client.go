package mrpc

import (
	"errors"
	"net"
	"sync/atomic"
)

var (
	// ErrClosed 表示连接已关闭。
	ErrClosed = errors.New("mrpc: connection has been closed")
	// ErrShutdown 表示客户端已手动关闭。
	ErrShutdown = errors.New("mrpc: client is shut down")
)

// Client 是 RPC 客户端，支持单向非流式调用。
//
// 典型用法：
//
//	client := mrpc.NewClient("localhost", 8080)
//	defer client.Close()
//	var reply ReplyType
//	err := client.Call("Service.Method", &Request{...}, &reply)
//
// Call 会自动维护 TCP 长连接并在首次调用或断线时重连。
type Client struct {
	address string
	port    int
	con     net.Conn
	codec   Codec
	seq     uint64
	closed  bool
}

// NewClient 创建一个 RPC 客户端。
//
// 参数：
//   - address: 服务端 IP 或主机名（支持 IPv4/IPv6）
//   - port:    服务端端口号
//
// 边界条件：
//   - address 为空时 net.Dial 会返回错误（在 Call 时暴露）
//   - port <= 0 或 >65535 时 net.Dial 会返回错误
func NewClient(address string, port int) *Client {
	return &Client{address: address, port: port, codec: NewMsgCodec()}
}

// NewClinet 是 NewClient 的别名，保留以兼容旧代码。
// 新代码请使用 NewClient。
func NewClinet(address string, port int) *Client {
	return NewClient(address, port)
}

// addr 返回 "host:port" 格式的地址字符串，正确处理 IPv6。
func (c *Client) addr() string {
	return net.JoinHostPort(c.address, itoa(c.port))
}

// Dial 建立到服务端的 TCP 连接。
//
// 若已有连接，先关闭旧连接再建立新连接。
// 多次调用 Dial 是安全的。
func (c *Client) Dial() error {
	if c.con != nil {
		c.con.Close()
		c.con = nil
	}
	con, err := net.Dial("tcp", c.addr())
	if err != nil {
		return err
	}
	c.con = con
	c.closed = false
	return nil
}

// Call 执行一次单向非流式 RPC 调用。
//
// 流程：编码 argv → 发送请求 → 接收响应 → 解码到 reply。
// 若尚未建立连接，会自动调用 Dial 建立 TCP 连接。
// 连接会在多次 Call 之间复用（长连接）。
//
// 参数：
//   - method: 方法名，格式 "ServiceName.MethodName"
//   - argv:   请求参数指针（msgpack 需要指针才能编码）
//   - reply:  响应参数指针，结果将解码到此
//
// 返回值：
//   - 成功时返回 nil
//   - 网络错误、编解码错误、服务端返回的错误均通过 error 返回
//
// 边界条件：
//   - argv 必须是指针或可序列化类型，否则 Encode 失败
//   - reply 必须是指针，否则 Decode 无法写入
//   - 并发调用 Call 不安全（共用同一连接），需要外部加锁
func (c *Client) Call(method string, argv any, reply any) error {
	if c.closed {
		return ErrShutdown
	}
	if c.con == nil {
		if err := c.Dial(); err != nil {
			return err
		}
	}

	buff, err := c.codec.Encode(argv)
	if err != nil {
		return err
	}

	seq := atomic.AddUint64(&c.seq, 1)
	err = SendRequest(c.con, NewRequest(method, seq, buff))
	if err != nil {
		// 发送失败，标记连接为不可用
		c.con.Close()
		c.con = nil
		return err
	}

	resp, err := ReceiveResponse(c.con)
	if err != nil {
		c.con.Close()
		c.con = nil
		return err
	}

	if resp.Error != "" {
		return errors.New(resp.Error)
	}

	if reply != nil && len(resp.Reply) > 0 {
		return c.codec.Decode(resp.Reply, reply)
	}

	return nil
}

// Close 关闭客户端连接并标记为 shut down 状态。
//
// 关闭后再次调用 Call 会返回 ErrShutdown。
// 对已关闭的客户端调用 Close 是安全的（幂等）。
func (c *Client) Close() error {
	c.closed = true
	if c.con != nil {
		err := c.con.Close()
		c.con = nil
		return err
	}
	return nil
}

// itoa 将整数转为字符串，避免 import fmt。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
