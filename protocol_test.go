package mrpc

import (
	"bytes"
	"testing"
)

// TestRequestEncodeDecode 验证 Request 的编码/解码往返一致性。
func TestRequestEncodeDecode(t *testing.T) {
	// 正常请求
	orig := NewRequest("Arith.Multiply", 42, []byte{1, 2, 3, 4})
	payload, err := orig.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var decoded Request
	if err := decoded.Decode(payload); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if decoded.ServiceMethod != orig.ServiceMethod {
		t.Fatalf("ServiceMethod: got %q, want %q", decoded.ServiceMethod, orig.ServiceMethod)
	}
	if decoded.Seq != orig.Seq {
		t.Fatalf("Seq: got %d, want %d", decoded.Seq, orig.Seq)
	}
	if !bytes.Equal(decoded.Argv, orig.Argv) {
		t.Fatalf("Argv: got %v, want %v", decoded.Argv, orig.Argv)
	}
}

// TestResponseEncodeDecode 验证 Response 的编码/解码往返一致性。
func TestResponseEncodeDecode(t *testing.T) {
	// 成功响应
	orig := NewResponse(99, []byte{5, 6, 7}, "")
	payload, err := orig.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var decoded Response
	if err := decoded.Decode(payload); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if decoded.Seq != orig.Seq {
		t.Fatalf("Seq: got %d, want %d", decoded.Seq, orig.Seq)
	}
	if !bytes.Equal(decoded.Reply, orig.Reply) {
		t.Fatalf("Reply: got %v, want %v", decoded.Reply, orig.Reply)
	}
	if decoded.Error != orig.Error {
		t.Fatalf("Error: got %q, want %q", decoded.Error, orig.Error)
	}

	// 错误响应
		errResp := NewResponse(1, nil, "something went wrong")
	payload2, err := errResp.Encode()
	if err != nil {
		t.Fatalf("Encode error response failed: %v", err)
	}
	var decoded2 Response
	if err := decoded2.Decode(payload2); err != nil {
		t.Fatalf("Decode error response failed: %v", err)
	}
	if decoded2.Error != "something went wrong" {
		t.Fatalf("Error: got %q, want %q", decoded2.Error, "something went wrong")
	}
	// 注意：len=0 的 []byte 在 encode 后 decode 得到空切片（非 nil），
	// msgpack 将此视为合法编码。
	if len(decoded2.Reply) != 0 {
		t.Fatalf("Reply: expected empty, got len=%d", len(decoded2.Reply))
	}
}

// TestRequestEmptyFields 验证空字段（空方法名、空参数、seq=0）的编解码。
func TestRequestEmptyFields(t *testing.T) {
	orig := NewRequest("", 0, nil)
	payload, err := orig.Encode()
	if err != nil {
		t.Fatalf("Encode empty request failed: %v", err)
	}
	var decoded Request
	if err := decoded.Decode(payload); err != nil {
		t.Fatalf("Decode empty request failed: %v", err)
	}
	if decoded.ServiceMethod != "" {
		t.Fatalf("ServiceMethod: got %q, want empty", decoded.ServiceMethod)
	}
	if decoded.Seq != 0 {
		t.Fatalf("Seq: got %d, want 0", decoded.Seq)
	}
	if decoded.Argv != nil && len(decoded.Argv) != 0 {
		t.Fatalf("Argv: got %v, want empty", decoded.Argv)
	}
}

// TestWriteReadFrame 验证帧协议的写入/读取往返。
func TestWriteReadFrame(t *testing.T) {
	payload := []byte("hello mrpc frame test")
	var buf bytes.Buffer

	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadFrame: got %q, want %q", got, payload)
	}
}

// TestWriteFrameExceedsMaxPayload 验证超大 payload 被拒绝。
func TestWriteFrameExceedsMaxPayload(t *testing.T) {
	// 创建一个超过 10MB 的 payload（不实际分配内存，只测试逻辑）
	// 由于 MaxPayloadSize=10MB，10MB+1 字节就应触发 ErrMaxPayload
	largePayload := make([]byte, MaxPayloadSize+1)
	var buf bytes.Buffer
	err := WriteFrame(&buf, largePayload)
	if err != ErrMaxPayload {
		t.Fatalf("expected ErrMaxPayload, got %v", err)
	}
}

// TestReadFrameExceedsMaxPayload 验证读取超大帧被拒绝。
func TestReadFrameExceedsMaxPayload(t *testing.T) {
	var buf bytes.Buffer
	// 写入一个声称长度超过 MaxPayloadSize 的帧头
	WriteFrameHeader(&buf, MaxPayloadSize+1)
	_, err := ReadFrame(&buf)
	if err != ErrMaxPayload {
		t.Fatalf("expected ErrMaxPayload, got %v", err)
	}
}

// WriteFrameHeader 写入帧长度头（用于测试只写头不写体）。
func WriteFrameHeader(w *bytes.Buffer, size uint32) {
	b := make([]byte, 4)
	// Use BigEndian
	b[0] = byte(size >> 24)
	b[1] = byte(size >> 16)
	b[2] = byte(size >> 8)
	b[3] = byte(size)
	w.Write(b)
}

// TestSendReceiveRequest 验证通过帧协议发送/接收 Request。
func TestSendReceiveRequest(t *testing.T) {
	var buf bytes.Buffer
	req := NewRequest("Test.Ping", 1, []byte("ping"))

	if err := SendRequest(&buf, req); err != nil {
		t.Fatalf("SendRequest failed: %v", err)
	}

	got, err := ReceiveRequest(&buf)
	if err != nil {
		t.Fatalf("ReceiveRequest failed: %v", err)
	}
	if got.ServiceMethod != "Test.Ping" {
		t.Fatalf("ServiceMethod: got %q, want Test.Ping", got.ServiceMethod)
	}
	if got.Seq != 1 {
		t.Fatalf("Seq: got %d, want 1", got.Seq)
	}
	if !bytes.Equal(got.Argv, []byte("ping")) {
		t.Fatalf("Argv: got %v, want [112 105 110 103]", got.Argv)
	}
}

// TestSendReceiveResponse 验证通过帧协议发送/接收 Response。
func TestSendReceiveResponse(t *testing.T) {
	var buf bytes.Buffer
	resp := NewResponse(42, []byte("pong"), "")

	if err := SendResponse(&buf, resp); err != nil {
		t.Fatalf("SendResponse failed: %v", err)
	}

	got, err := ReceiveResponse(&buf)
	if err != nil {
		t.Fatalf("ReceiveResponse failed: %v", err)
	}
	if got.Seq != 42 {
		t.Fatalf("Seq: got %d, want 42", got.Seq)
	}
	if !bytes.Equal(got.Reply, []byte("pong")) {
		t.Fatalf("Reply: got %v, want %v", got.Reply, []byte("pong"))
	}
	if got.Error != "" {
		t.Fatalf("Error: got %q, want empty", got.Error)
	}
}

// TestRequestZeroSeq 验证 seq=0 的请求正常工作（序号的边界值）。
func TestRequestZeroSeq(t *testing.T) {
	req := NewRequest("Svc.M", 0, nil)
	payload, err := req.Encode()
	if err != nil {
		t.Fatalf("Encode seq=0 failed: %v", err)
	}
	var decoded Request
	if err := decoded.Decode(payload); err != nil {
		t.Fatalf("Decode seq=0 failed: %v", err)
	}
	if decoded.Seq != 0 {
		t.Fatalf("Seq: got %d, want 0", decoded.Seq)
	}
}

// TestRequestMaxUint64Seq 验证 seq=MaxUint64 时编解码正确。
func TestRequestMaxUint64Seq(t *testing.T) {
	req := NewRequest("Svc.M", ^uint64(0), nil)
	payload, err := req.Encode()
	if err != nil {
		t.Fatalf("Encode max seq failed: %v", err)
	}
	var decoded Request
	if err := decoded.Decode(payload); err != nil {
		t.Fatalf("Decode max seq failed: %v", err)
	}
	if decoded.Seq != ^uint64(0) {
		t.Fatalf("Seq: got %d, want %d", decoded.Seq, ^uint64(0))
	}
}

// TestResponseWithErrorMessage 验证错误消息中包含特殊字符。
func TestResponseWithErrorMessage(t *testing.T) {
	errMsg := "error: something\nwent\twrong"
	resp := NewResponse(1, nil, errMsg)
	payload, err := resp.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	var decoded Response
	if err := decoded.Decode(payload); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if decoded.Error != errMsg {
		t.Fatalf("Error: got %q, want %q", decoded.Error, errMsg)
	}
}
