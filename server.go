package mrpc

import "net"

type Server struct{
	 lis net.Listener
}
type Connect struct{
	 
}
func NewServer(lis net.Listener)*Server{
	 return &Server{lis: lis}
}
func (s*Server)Run(){
	 
}
func (s*Server)Register(name string){
	
}
