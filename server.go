package mrpc

import (
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
)

// RPCMethod 描述找到的符合 func(Req, *Reply) error 签名的方法
type RPCMethod struct {
	ReqType   reflect.Type // 第1个参数（request）类型
	ReplyType reflect.Type // 第2个参数（reply）类型，已保证为指针
}

type Server struct {
	lis     net.Listener
	svc     map[string]reflect.Value
	methods map[string]RPCMethod
}
type Connect struct {
	s     *Server
	con   net.Conn
	codec Codec
}

func NewServer(lis net.Listener) *Server {
	if lis == nil {
		return nil
	}
	return &Server{lis: lis}
}
func (s *Server) NewConn(con net.Conn, codec Codec) *Connect {
	if con != nil {
		return nil
	}
	return &Connect{
		s:     s,
		con:   con,
		codec: codec,
	}
}
func (s *Server) Run() {
	defer s.lis.Close()
	for {
		con, err := s.lis.Accept()
		if err != nil {
			log.Println(err)
		}
		go s.NewConn(con, &MsgCodec{}).Handler()
	}
}
func (c *Connect) Handler() {
	defer c.con.Close()
	rq, err := ReceiveRequest(c.con)
	if err != nil {
		log.Println(err)
		return
	}
	method, ok := c.s.methods[rq.ServiceMethod]
	if !ok {
		return
	}
	req := reflect.New(method.ReqType).Interface()
	reply := reflect.New(method.ReqType).Interface()
	err = c.codec.Decode(rq.Argv, req)
	if err != nil {
		log.Println(err)
		return
	}
	cerr := c.s.Call(rq.ServiceMethod, req, reply)
	buff, err := c.codec.Encode(reply)
	if err != nil {
		log.Println(err)
		return
	}
	SendResponse(c.con, NewResponse(0, buff, cerr.Error()))

}

// Registery 从结构体指针中找出所有签名类似 func(request, *reply) error 的方法
func (s *Server) Registery(name string, target any) error {
	t := reflect.TypeOf(target)

	if t.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer, got %v", t.Kind())
	}
	if t.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("target must point to a struct, got %v", t.Elem().Kind())
	}

	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		mt := method.Type

		// 参数数量：receiver + req + reply = 3
		if mt.NumIn() != 3 {
			continue
		}
		// 返回值：恰好1个，且为 error
		if mt.NumOut() != 1 || mt.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		// ✅ reply 必须是指针
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
func (s *Server) Call(method string, req, reply any) error {
	sn, mn, found := strings.Cut(method, ".")
	if found {
		return errors.New("Method Format Error")
	}
	v := s.svc[sn]
	return CallMethod(v, mn, req, reply)
}

// CallMethod 调用指定名称的 RPC 风格方法
func CallMethod(target reflect.Value, methodName string, req, reply any) error {
	v := reflect.ValueOf(target)
	m := v.MethodByName(methodName)
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
