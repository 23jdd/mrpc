package mrpc

import "github.com/vmihailenco/msgpack"
type Codec interface {
    Encode(v any) ([]byte, error)
    Decode(data []byte, v any) error
}


type MsgCodec struct{
	 
}
func NewMsgCodec()*MsgCodec{
    return &MsgCodec{}
}
func (mc*MsgCodec)Encode(v any)([]byte,error){
      return msgpack.Marshal(v)      
}
func (mc*MsgCodec)Decode(data []byte,v any)error{
	  return msgpack.Unmarshal(data,v) 
}
