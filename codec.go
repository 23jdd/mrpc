package mrpc

import "github.com/vmihailenco/msgpack"

// Codec 定义编解码器接口，负责请求/响应体的序列化与反序列化。
//
// 实现者需保证 Encode/Decode 的线程安全性（本库在服务端每个连接
// 使用独立 Codec 实例，客户端单连接复用同一实例）。
type Codec interface {
	Encode(v any) ([]byte, error)
	Decode(data []byte, v any) error
}

// MsgCodec 是基于 msgpack 的 Codec 实现。
type MsgCodec struct{}

// NewMsgCodec 创建一个新的 MsgCodec 实例。
func NewMsgCodec() *MsgCodec {
	return &MsgCodec{}
}

// Encode 使用 msgpack 序列化 v。
func (mc *MsgCodec) Encode(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}

// Decode 使用 msgpack 反序列化 data 到 v。
func (mc *MsgCodec) Decode(data []byte, v any) error {
	return msgpack.Unmarshal(data, v)
}
