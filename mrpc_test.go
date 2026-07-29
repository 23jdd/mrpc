package mrpc

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// TestEndToEnd 验证完整的 Client → Server 单向非流式 RPC 调用。
func TestEndToEnd(t *testing.T) {
	srv, host, port := startTestServer(t)
	defer srv.lis.Close()

	err := srv.Register("Arith", &testArith{})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	err = srv.Register("Health", RegisterHealth())
	if err != nil {
		t.Fatalf("Register health: %v", err)
	}

	go srv.Run()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(host, port)
	defer client.Close()

	var reply AddReply
	err = client.Call("Arith.Add", &AddReq{A: 10, B: 20}, &reply)
	if err != nil {
		t.Fatalf("Call Arith.Add: %v", err)
	}
	if reply.Sum != 30 {
		t.Fatalf("Sum: got %d, want 30", reply.Sum)
	}
}

// TestEndToEndMultipleCalls 验证同一连接上的多次调用（连接复用）。
func TestEndToEndMultipleCalls(t *testing.T) {
	srv, host, port := startTestServer(t)
	defer srv.lis.Close()

	srv.Register("Arith", &testArith{})

	go srv.Run()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(host, port)
	defer client.Close()

	for i := 0; i < 5; i++ {
		var reply AddReply
		err := client.Call("Arith.Add", &AddReq{A: i, B: i * 2}, &reply)
		if err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
		if reply.Sum != i+i*2 {
			t.Fatalf("Call %d Sum: got %d, want %d", i, reply.Sum, i+i*2)
		}
	}
}

// TestEndToEndMethodReturnsError 验证服务端方法返回错误时客户端能正确收到。
func TestEndToEndMethodReturnsError(t *testing.T) {
	srv, host, port := startTestServer(t)
	defer srv.lis.Close()
	srv.Register("ErrSvc", &errService{})

	go srv.Run()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(host, port)
	defer client.Close()

	var reply AddReply
	err := client.Call("ErrSvc.Fail", &AddReq{A: 1, B: 2}, &reply)
	if err == nil {
		t.Fatal("expected error from service, got nil")
	}
	if err.Error() != "service error: out of range" {
		t.Fatalf("error message: got %q, want %q", err.Error(), "service error: out of range")
	}
}

// TestEndToEndUnknownMethod 验证调用未注册方法时的行为。
func TestEndToEndUnknownMethod(t *testing.T) {
	srv, host, port := startTestServer(t)
	defer srv.lis.Close()
	srv.Register("Arith", &testArith{})

	go srv.Run()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(host, port)
	defer client.Close()

	var reply AddReply
	err := client.Call("Arith.UnknownMethod", &AddReq{A: 1, B: 2}, &reply)
	if err == nil {
		t.Fatal("expected error for unknown method, got nil")
	}
}

// TestEndToEndHealthCheck 验证健康检查端到端调用。
func TestEndToEndHealthCheck(t *testing.T) {
	srv, host, port := startTestServer(t)
	defer srv.lis.Close()
	srv.Register("Health", RegisterHealth())

	go srv.Run()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(host, port)
	defer client.Close()

	var reply HealthReply
	err := client.Call("Health.Check", &HealthRequest{}, &reply)
	if err != nil {
		t.Fatalf("Health.Check: %v", err)
	}
	if !reply.Ok {
		t.Fatal("Health.Check: expected Ok=true")
	}
}

// TestClientClosed 验证客户端关闭后调用 Call 返回 ErrShutdown。
func TestClientClosed(t *testing.T) {
	client := NewClient("localhost", 19999)
	client.Close()

	var reply AddReply
	err := client.Call("Arith.Add", &AddReq{}, &reply)
	if err != ErrShutdown {
		t.Fatalf("expected ErrShutdown after Close, got %v", err)
	}
}

// TestClientDialFailure 验证连接失败时 Call 返回错误。
func TestClientDialFailure(t *testing.T) {
	client := NewClient("127.0.0.1", 19998)
	var reply AddReply
	err := client.Call("Arith.Add", &AddReq{}, &reply)
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

// TestEndToEndPointerRequestType 验证请求参数为指针类型的端到端调用。
func TestEndToEndPointerRequestType(t *testing.T) {
	srv, host, port := startTestServer(t)
	defer srv.lis.Close()
	srv.Register("Arith", &testArith{})

	go srv.Run()
	time.Sleep(50 * time.Millisecond)

	client := NewClient(host, port)
	defer client.Close()

	var reply AddReply
	err := client.Call("Arith.EchoReq", &AddReq{A: 4, B: 6}, &reply)
	if err != nil {
		t.Fatalf("Call EchoReq: %v", err)
	}
	if reply.Sum != 10 {
		t.Fatalf("Sum: got %d, want 10", reply.Sum)
	}
}

// errService 用于测试服务端返回错误的场景。
type errService struct{}

func (e *errService) Fail(req AddReq, reply *AddReply) error {
	return &testRPCError{msg: "service error: out of range"}
}

// startTestServer 创建一个监听随机端口的测试服务端。
// 返回 Server、主机名和端口号。
func startTestServer(t *testing.T) (*Server, string, int) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	host, portStr, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	return NewServer(lis), host, port
}
