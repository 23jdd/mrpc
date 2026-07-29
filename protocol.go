package mrpc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

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

func NewRequest(method string, seq uint64, argv []byte) *Request {
	return &Request{ServiceMethod: method, Seq: seq, Argv: argv}
}
func NewResponse(seq uint64, reply []byte, err string) *Response {
	return &Response{Seq: seq, Reply: reply, Error: err}
}

// Encode
func (re *Request) Encode() ([]byte, error) {
	var buf bytes.Buffer

	if err := writeString(&buf, re.ServiceMethod); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.BigEndian, re.Seq); err != nil {
		return nil, err
	}
	if err := writeBytes(&buf, re.Argv); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (re *Request) Decode(data []byte) error {
	r := bytes.NewReader(data)

	method, err := readString(r)
	if err != nil {
		return err
	}
	re.ServiceMethod = method

	if err := binary.Read(r, binary.BigEndian, &re.Seq); err != nil {
		return err
	}

	argv, err := readBytes(r)
	if err != nil {
		return err
	}
	re.Argv = argv
	return nil
}

func (rs *Response) Encode() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.BigEndian, rs.Seq); err != nil {
		return nil, err
	}
	if err := writeBytes(&buf, rs.Reply); err != nil {
		return nil, err
	}
	if err := writeString(&buf, rs.Error); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (rs *Response) Decode(data []byte) error {
	r := bytes.NewReader(data)

	if err := binary.Read(r, binary.BigEndian, &rs.Seq); err != nil {
		return err
	}

	reply, err := readBytes(r)
	if err != nil {
		return err
	}
	rs.Reply = reply

	errStr, err := readString(r)
	if err != nil {
		return err
	}
	rs.Error = errStr
	return nil
}

func writeString(buf *bytes.Buffer, s string) error {
	if err := binary.Write(buf, binary.BigEndian, uint32(len(s))); err != nil {
		return err
	}
	_, err := buf.WriteString(s)
	return err
}

func readString(r io.Reader) (string, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	b := DefaultPool.Get(int(length))
	if _, err := io.ReadFull(r, b); err != nil {
		DefaultPool.Put(b)
		return "", err
	}
	s := string(b)
	DefaultPool.Put(b)
	return s, nil
}

func writeBytes(buf *bytes.Buffer, b []byte) error {
	if err := binary.Write(buf, binary.BigEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err := buf.Write(b)
	return err
}

func readBytes(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

var ErrMaxPayload = errors.New("mrpc: payload exceeds maximum size")

const MaxPayloadSize = 10 << 20 // 10 MB

func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxPayloadSize {
		return ErrMaxPayload
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func ReadFrame(r io.Reader) ([]byte, error) {
	var totalLen uint32
	if err := binary.Read(r, binary.BigEndian, &totalLen); err != nil {
		return nil, err
	}
	if totalLen > MaxPayloadSize {
		return nil, ErrMaxPayload
	}
	payload := DefaultPool.Get(int(totalLen))
	if _, err := io.ReadFull(r, payload); err != nil {
		DefaultPool.Put(payload)
		return nil, err
	}
	return payload, nil
}

// PutFrame 将从 ReadFrame 获取的帧 buffer 归还给 DefaultPool。
// 调用方在完成帧数据的解码后应调用此函数。
// 与 DefaultPool.Put 等价，方便配对使用：ReadFrame / PutFrame。
func PutFrame(payload []byte) {
	DefaultPool.Put(payload)
}

func SendRequest(w io.Writer, req *Request) error {
	payload, err := req.Encode()
	if err != nil {
		return err
	}
	return WriteFrame(w, payload)
}

func ReceiveRequest(r io.Reader) (*Request, error) {
	payload, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	defer PutFrame(payload)
	var req Request
	if err := req.Decode(payload); err != nil {
		return nil, err
	}
	return &req, nil
}

func SendResponse(w io.Writer, resp *Response) error {
	payload, err := resp.Encode()
	if err != nil {
		return err
	}
	return WriteFrame(w, payload)
}

func ReceiveResponse(r io.Reader) (*Response, error) {
	payload, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	defer PutFrame(payload)
	var resp Response
	if err := resp.Decode(payload); err != nil {
		return nil, err
	}
	return &resp, nil
}
