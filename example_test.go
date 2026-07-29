package mrpc_test

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/23jdd/mrpc"
)

// ------------------------------- 定义服务 -------------------------------

// Calculator 是一个示例 RPC 服务实现。
type Calculator struct{}

// MultiplyReq 乘法请求参数。
// 字段首字母必须大写（msgpack 序列化要求）。
type MultiplyReq struct {
	A int // 乘数 A
	B int // 乘数 B
}

// MultiplyReply 乘法响应结果。
type MultiplyReply struct {
	Product int // A * B 的结果
}

// Multiply 乘法 RPC 方法。
//
// 签名必须满足 func(req T, reply *U) error。
//   - req:  请求参数（可以是值类型或指针类型，msgpack 可反序列化即可）
//   - reply: 响应参数，必须为指针（以便 server 将结果写入）
//   - error: 方法执行错误，为 nil 时表示成功
//
// 边界条件：
//   - req 类型错误时 msgpack 解码会失败
//   - reply 为 nil 时会导致 panic
//   - 方法中若 panic 未被 recover，会导致连接关闭
func (c *Calculator) Multiply(req MultiplyReq, reply *MultiplyReply) error {
	// 注意：溢出和除零等业务边界需自行处理，此处简化
	reply.Product = req.A * req.B
	return nil
}

// ------------------------------- 示例：完整 Client/Server -------------------------------

// Example_serverAndClient 演示从服务注册到客户端调用的完整流程。
//
// 流程：
//  1. 创建 net.Listener 监听 TCP 端口
//  2. 创建 Server 并注册服务实现
//  3. 在后台 goroutine 启动服务端
//  4. 创建 Client 发起 RPC 调用
//  5. 解析响应并打印结果
//
// 注意事项：
//   - 单向非流式：每次调用发一个请求、收一个响应，连接随之关闭（server 端）
//   - Client 默认使用 msgpack 编解码，连接在多次 Call 之间复用
//   - Call 的 argv 和 reply 参数都必须为指针，否则编解码失败
func Example_serverAndClient() {
	// 1. 创建 TCP listener
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer lis.Close()

	// 2. 创建 Server 并注册服务
	server := mrpc.NewServer(lis)
	if err := server.Register("Calculator", &Calculator{}); err != nil {
		log.Fatal(err)
	}

	// 3. 后台启动服务端
	go server.Run()

	// 4. 创建客户端（连接服务端监听的端口）
	_, port, _ := net.SplitHostPort(lis.Addr().String())
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)

	client := mrpc.NewClient("127.0.0.1", portNum)
	defer client.Close()

	// 给服务端一点时间
	time.Sleep(50 * time.Millisecond)

	// 5. 发起 RPC 调用
	req := &MultiplyReq{A: 7, B: 8}
	var reply MultiplyReply
	if err := client.Call("Calculator.Multiply", req, &reply); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%d * %d = %d\n", req.A, req.B, reply.Product)
	// Output:
	// 7 * 8 = 56
}

// ------------------------------- 示例：错误处理 -------------------------------

// Example_errorHandling 演示各种错误场景的处理。
func Example_errorHandling() {
	lis, _ := net.Listen("tcp", "127.0.0.1:0")
	defer lis.Close()

	server := mrpc.NewServer(lis)
	server.Register("Calculator", &Calculator{})
	go server.Run()

	_, port, _ := net.SplitHostPort(lis.Addr().String())
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)

	client := mrpc.NewClient("127.0.0.1", portNum)
	defer client.Close()
	time.Sleep(50 * time.Millisecond)

	// 场景1：调用不存在的方法 → 服务端关闭连接，客户端收到网络错误
	var reply MultiplyReply
	err := client.Call("Calculator.NoSuchMethod", &MultiplyReq{}, &reply)
	if err != nil {
		fmt.Println("unknown method:", err != nil)
	}

	// 场景2：调用不存在的服务 → 同上
	err = client.Call("NoService.Method", &MultiplyReq{}, &reply)
	if err != nil {
		fmt.Println("unknown service:", err != nil)
	}

	// 场景3：传入 nil reply → msgpack 无法解码，通常不会有问题（nil reply 跳过解码）
	err = client.Call("Calculator.Multiply", &MultiplyReq{A: 1, B: 2}, nil)
	fmt.Println("nil reply, err:", err)

	// Output:
	// unknown method: true
	// unknown service: true
	// nil reply, err: <nil>
}

// ------------------------------- 示例：健康检查 -------------------------------

// Example_healthCheck 演示健康检查功能。
func Example_healthCheck() {
	lis, _ := net.Listen("tcp", "127.0.0.1:0")
	defer lis.Close()

	server := mrpc.NewServer(lis)
	server.Register("Calculator", &Calculator{})
	// 注册健康检查服务
	server.Register("Health", mrpc.RegisterHealth())
	go server.Run()

	_, port, _ := net.SplitHostPort(lis.Addr().String())
	var portNum int
	fmt.Sscanf(port, "%d", &portNum)

	client := mrpc.NewClient("127.0.0.1", portNum)
	defer client.Close()
	time.Sleep(50 * time.Millisecond)

	// 调用健康检查
	var healthReply mrpc.HealthReply
	err := client.Call("Health.Check", &mrpc.HealthRequest{}, &healthReply)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("server healthy:", healthReply.Ok)

	// 启动周期性健康检查（后台 goroutine）
	checker := mrpc.NewHealthChecker(5*time.Second, 2*time.Second)
	checker.Start(client, 3, func(err error) {
		fmt.Println("health check failed:", err)
	})
	defer checker.Stop()

	// Output:
	// server healthy: true
}
