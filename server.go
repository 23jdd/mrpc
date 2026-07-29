package mrpc

import (
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
	"sync/atomic"
)

// RPCMethod 描述找到的符合 func(Req, *Reply) error 签名的方法。
// ReqType 为请求参数类型，ReplyType 为响应参数类型（已保证为指针）。
type RPCMethod struct {
	ReqType   reflect.Type
	ReplyType reflect.Type
}

// Server 是一个基于 TCP 的反射型 RPC 服务端。
// 通过 Register 注册服务实现，Run 启动监听循环，对每个连接
// 在独立 goroutine 中处理单次单向非流式调用。
type Server struct {
	lis     net.Listener
	svc     map[string]reflect.Value
	methods map[string]RPCMethod
	seq     uint64
}

// Connect 表示一个已建立的客户端连接，绑定特定编解码器。
type connect struct {
	s     *Server
	con   net.Conn
	codec Codec
}

// NewServer 创建一个 RPC 服务端，绑定到给定的 net.Listener。
// lis 必须非 nil，否则会在 Run 时 panic。
func NewServer(lis net.Listener) *Server {
	return &Server{
		lis:     lis,
		svc:     make(map[string]reflect.Value),
		methods: make(map[string]RPCMethod),
	}
}

// nextSeq 返回下一个自增请求序号，用于匹配请求与响应。
func (s *Server) nextSeq() uint64 {
	return atomic.AddUint64(&s.seq, 1)
}

// NewConn 从 TCP 连接创建一个 Connect 对象。
func (s *Server) NewConn(con net.Conn, codec Codec) *connect {
	if con == nil {
		return nil
	}
	return &connect{
		s:     s,
		con:   con,
		codec: codec,
	}
}

// Run 启动服务端主循环：在调用方 goroutine 中阻塞 Accept 新连接，
// 并为每个连接启动一个新 goroutine 调用 Handler。
// 当 lis.Accept 返回 net.ErrClosed 或其它不可恢复错误时退出。
func (s *Server) Run() {
	if s.lis == nil {
		panic("mrpc: Server.Run called with nil listener")
	}
	defer s.lis.Close()
	for {
		con, err := s.lis.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// 对于临时错误（如 Accept 超时），在 v1 中跳过并继续。
			// 生产环境建议使用临时错误判断 + 有限次重试。
			log.Println("mrpc: accept error:", err)
			continue
		}
		conn := s.NewConn(con, NewMsgCodec())
		if conn == nil {
			con.Close()
			continue
		}
		go conn.Handler()
	}
}

// Handler 处理 RPC 请求循环：在一个连接上可处理多次单向非流式调用。
// 每次循环：接收一个请求 → 反射调用已注册方法 → 发送响应。
// 当 ReadRequest 返回 io.EOF 或其它不可恢复错误时退出并关闭连接。
func (c *connect) Handler() {
	defer c.con.Close()

	for {
		rq, err := ReceiveRequest(c.con)
		if err != nil {
			if err.Error() != "EOF" {
				log.Println("mrpc: receive request:", err)
			}
			return
		}

		method, ok := c.s.methods[rq.ServiceMethod]
		if !ok {
			log.Printf("mrpc: method %s not registered", rq.ServiceMethod)
			return
		}

		// 为解码创建请求类型的指针（msgpack 需要指针才能反序列化）
		// 区分 ReqType 是指针类型（如 *Foo）还是值类型（如 Foo）
		var reqPtrVal reflect.Value
		if method.ReqType.Kind() == reflect.Ptr {
			reqPtrVal = reflect.New(method.ReqType.Elem()) // *Foo
		} else {
			reqPtrVal = reflect.New(method.ReqType) // *Foo（对值类型 Foo 而言）
		}
		reqPtr := reqPtrVal.Interface()

		if err = c.codec.Decode(rq.Argv, reqPtr); err != nil {
			log.Println("mrpc: decode request:", err)
			return
		}

		// 取得方法签名所需的精确匹配类型
		var req any
		if method.ReqType.Kind() == reflect.Ptr {
			req = reqPtr // 已经是指针，与方法签名匹配
		} else {
			req = reqPtrVal.Elem().Interface() // 解引用为值类型
		}

		// 创建响应参数：ReplyType 必定为指针类型
		reply := reflect.New(method.ReplyType.Elem()).Interface()

		cerr := c.s.call(rq.ServiceMethod, req, reply)
		if cerr != nil {
			SendResponse(c.con, NewResponse(0, nil, cerr.Error()))
			return
		}

		buff, err := c.codec.Encode(reply)
		if err != nil {
			log.Println("mrpc: encode response:", err)
			return
		}
		if err := SendResponse(c.con, NewResponse(0, buff, "")); err != nil {
			log.Println("mrpc: send response:", err)
			return
		}
	}
}

// Register 从结构体指针中找出所有签名类似 func(request, *reply) error 的方法并注册。
//
// 参数：
//   - name:     服务名，调用时使用 "name.MethodName" 格式
//   - target:   服务实现，必须为指向 struct 的指针
//
// 边界条件：
//   - target 必须是 *struct 指针，否则返回错误
//   - 只注册满足 func(req T, reply *U) error 签名的方法
//   - 同名服务重复注册会覆盖之前的方法
//   - 空结构体或无符合方法时不会报错（直接返回 nil）
func (s *Server) Register(name string, target any) error {
	t := reflect.TypeOf(target)

	if t.Kind() != reflect.Ptr {
		return fmt.Errorf("mrpc: target must be a pointer to struct, got %v", t.Kind())
	}
	if t.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("mrpc: target must point to a struct, got %v", t.Elem().Kind())
	}

	// 若 name 之前已注册，先清理旧方法
	if s.methods != nil {
		for k := range s.methods {
			if strings.HasPrefix(k, name+".") {
				delete(s.methods, k)
			}
		}
	}
	if s.methods == nil {
		s.methods = make(map[string]RPCMethod)
	}
	if s.svc == nil {
		s.svc = make(map[string]reflect.Value)
	}

	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		mt := method.Type

		// 参数数量：receiver + req + reply = 3
		if mt.NumIn() != 3 {
			continue
		}
		// 返回值：恰好 1 个，且为 error
		if mt.NumOut() != 1 || mt.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		// reply 必须是指针
		if mt.In(2).Kind() != reflect.Ptr {
			continue
		}
		s.methods[name+"."+method.Name] = RPCMethod{
			ReqType:   mt.In(1),
			ReplyType: mt.In(2),
		}
	}
	s.svc[name] = reflect.ValueOf(target)

	return nil
}

// Registery 是 Register 的别名，保留以兼容旧代码。
// 新代码请使用 Register。
func (s *Server) Registery(name string, target any) error {
	return s.Register(name, target)
}

// call 根据 "ServiceName.MethodName" 格式的方法名进行反射调用。
//
// 参数：
//   - method: 格式为 "ServiceName.MethodName"
//   - req:    请求参数，类型必须与注册时一致
//   - reply:  响应参数指针，结果将写入此处
func (s *Server) call(method string, req, reply any) error {
	svcName, methName, found := strings.Cut(method, ".")
	if !found {
		return errors.New("mrpc: method format error, expected 'Service.Method'")
	}
	v, ok := s.svc[svcName]
	if !ok {
		return fmt.Errorf("mrpc: service %s not found", svcName)
	}
	return callMethod(v, methName, req, reply)
}

// callMethod 对指定反射值调用其指定名称的方法。
//
// target 必须是合法的 reflect.Value（直接使用 s.svc 中的值），
// 不可再用 reflect.ValueOf 二次包装。
//
// 参数：
//   - target:     reflect.Value，指向已注册的服务实现
//   - methodName: 方法名
//   - req:        请求参数（类型必须与方法签名的第一个参数匹配）
//   - reply:      响应参数指针（类型必须与方法签名的第二个参数匹配）
//
// 边界条件：
//   - 若方法名不存在，reflect.MethodByName 返回 zero Value，Call 会 panic
//   - req/reply 类型不匹配时，reflect.Call 会 panic
func callMethod(target reflect.Value, methodName string, req, reply any) error {
	m := target.MethodByName(methodName)
	args := []reflect.Value{
		reflect.ValueOf(req),
		reflect.ValueOf(reply),
	}
	out := m.Call(args)
	if !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}
