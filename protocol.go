package mrpc
type Request struct {
    ServiceMethod string // "Calculator.Add"
    Seq           uint64 // 请求 ID，用于异步匹配
    Argv          []byte // 序列化后的参数
}

type Response struct {
    Seq   uint64 // 对应请求 ID
    Reply []byte // 序列化后的返回值
    Error string // 错误信息
}
//  TL
func (re*Request)Encode(){
	 
}
func (re*Request)Decode(){
	  
}
func (rs*Response)Encode(){
	 
}
func (rs*Response)Decode(){
	 
}