package mrpc

import (
	"reflect"
	"testing"
)

// testArith 是一个用于测试的算术服务。
type testArith struct{}

type AddReq struct {
	A, B int
}
type AddReply struct {
	Sum int
}

func (a *testArith) Add(req AddReq, reply *AddReply) error {
	reply.Sum = req.A + req.B
	return nil
}

func (a *testArith) EchoReq(req *AddReq, reply *AddReply) error {
	reply.Sum = req.A + req.B
	return nil
}

// testMath 用于测试多服务注册及签名过滤。
type testMath struct{}

func (m *testMath) Mult(a, b int) int { return a * b }

func (m *testMath) Div(req AddReq, reply *AddReply) error {
	reply.Sum = req.A - req.B
	return nil
}

// testFoo 用于测试重复注册覆盖。
type testFoo struct{}

func (f *testFoo) Do(req AddReq, reply *AddReply) error {
	reply.Sum = 999
	return nil
}

// testErr 用于测试方法返回错误。
type testErr struct{}

func (e *testErr) Fail(req AddReq, reply *AddReply) error {
	return &testRPCError{msg: "intentional failure"}
}

// TestServerRegister 验证服务注册逻辑。
func TestServerRegister(t *testing.T) {
	srv := NewServer(nil)

	err := srv.Register("Arith", &testArith{})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if _, ok := srv.methods["Arith.Add"]; !ok {
		t.Fatal("method Arith.Add not found in methods map")
	}
	if _, ok := srv.svc["Arith"]; !ok {
		t.Fatal("service Arith not found in svc map")
	}

	// 注册非指针类型应报错
	err = srv.Register("Bad", testArith{})
	if err == nil {
		t.Fatal("expected error when registering non-pointer type")
	}

	// 注册指向非结构体的指针应报错
	var x int
	err = srv.Register("Bad2", &x)
	if err == nil {
		t.Fatal("expected error when registering pointer to non-struct")
	}
}

// TestServerRegisterMultipleServices 验证多个服务注册不冲突，且非法签名被过滤。
func TestServerRegisterMultipleServices(t *testing.T) {
	srv := NewServer(nil)
	err := srv.Register("Arith", &testArith{})
	if err != nil {
		t.Fatalf("Register Arith failed: %v", err)
	}
	err = srv.Register("Math", &testMath{})
	if err != nil {
		t.Fatalf("Register Math failed: %v", err)
	}

	if len(srv.methods) != 3 {
		t.Fatalf("expected 3 methods (Arith.Add + Arith.EchoReq + Math.Div), got %d", len(srv.methods))
	}
	if _, ok := srv.methods["Arith.Add"]; !ok {
		t.Fatal("Arith.Add not registered")
	}
	if _, ok := srv.methods["Math.Div"]; !ok {
		t.Fatal("Math.Div not registered")
	}
	// Mult 不满足签名，不应被注册
	if _, ok := srv.methods["Math.Mult"]; ok {
		t.Fatal("Math.Mult should not be registered (wrong signature)")
	}
}

// TestServerRegisterCover 验证重复注册同名服务会覆盖。
func TestServerRegisterCover(t *testing.T) {
	srv := NewServer(nil)
	srv.Register("Svc", &testArith{})
	srv.Register("Svc", &testFoo{})

	if len(srv.methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(srv.methods))
	}
}

// TestCallMethod 验证反射调用方法。
func TestCallMethod(t *testing.T) {
	a := &testArith{}
	v := reflect.ValueOf(a)

	req := AddReq{A: 3, B: 5}
	var reply AddReply

	err := callMethod(v, "Add", req, &reply)
	if err != nil {
		t.Fatalf("callMethod failed: %v", err)
	}
	if reply.Sum != 8 {
		t.Fatalf("Sum: got %d, want 8", reply.Sum)
	}
}

// TestCallMethodError 验证方法返回错误时正确处理。
func TestCallMethodError(t *testing.T) {
	v := reflect.ValueOf(&testErr{})
	var reply AddReply
	err := callMethod(v, "Fail", AddReq{}, &reply)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "intentional failure" {
		t.Fatalf("error message: got %q, want %q", err.Error(), "intentional failure")
	}
}

// TestServerCall 验证 Server.Call 的路由分发逻辑。
func TestServerCall(t *testing.T) {
	srv := NewServer(nil)
	srv.Register("Arith", &testArith{})

	var reply AddReply
	err := srv.Call("Arith.Add", AddReq{A: 2, B: 3}, &reply)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if reply.Sum != 5 {
		t.Fatalf("Sum: got %d, want 5", reply.Sum)
	}

	// 错误格式的方法名
	err = srv.Call("BadFormat", AddReq{}, &reply)
	if err == nil {
		t.Fatal("expected error for malformed method name")
	}

	// 不存在的服务
	err = srv.Call("NoSuch.Service", AddReq{}, &reply)
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
}

// TestServerNewConnNil 验证 NewConn 对 nil 连接返回 nil。
func TestServerNewConnNil(t *testing.T) {
	srv := NewServer(nil)
	conn := srv.NewConn(nil, NewMsgCodec())
	if conn != nil {
		t.Fatal("NewConn with nil should return nil")
	}
}

// TestRegisterHealth 验证健康检查服务注册。
func TestRegisterHealth(t *testing.T) {
	srv := NewServer(nil)
	err := srv.Register("Health", RegisterHealth())
	if err != nil {
		t.Fatalf("Register health failed: %v", err)
	}
	if _, ok := srv.methods["Health.Check"]; !ok {
		t.Fatal("Health.Check not registered")
	}
}

// TestServerRegisterNilMaps 验证在未初始化的 Server 上注册也能 lazy init maps。
func TestServerRegisterNilMaps(t *testing.T) {
	srv := &Server{}
	err := srv.Register("Svc", &testArith{})
	if err != nil {
		t.Fatalf("Register on nil maps should lazily init: %v", err)
	}
	if srv.methods == nil {
		t.Fatal("methods map should be initialized")
	}
	if srv.svc == nil {
		t.Fatal("svc map should be initialized")
	}
}

// TestServerCallPointerReqType 验证请求参数为指针类型时的正确行为。
func TestServerCallPointerReqType(t *testing.T) {
	srv := NewServer(nil)
	srv.Register("Arith", &testArith{})

	var reply AddReply
	req := &AddReq{A: 7, B: 3}
	err := srv.Call("Arith.EchoReq", req, &reply)
	if err != nil {
		t.Fatalf("Call EchoReq failed: %v", err)
	}
	if reply.Sum != 10 {
		t.Fatalf("Sum: got %d, want 10", reply.Sum)
	}
}

// TestRegisteryBackwardCompat 验证旧 API Registery 仍然可用。
func TestRegisteryBackwardCompat(t *testing.T) {
	srv := NewServer(nil)
	err := srv.Registery("Arith", &testArith{})
	if err != nil {
		t.Fatalf("Registery failed: %v", err)
	}
	if _, ok := srv.methods["Arith.Add"]; !ok {
		t.Fatal("Arith.Add not found")
	}
}

type testRPCError struct {
	msg string
}

func (e *testRPCError) Error() string {
	return e.msg
}
