package mrpc

import (
	"errors"
	"fmt"
	"net"
)

var (
	CloseError = errors.New("Connect has been closed")
)

type Client struct {
	address string
	port    int
	con     net.Conn
	codec   Codec
}

func NewClinet(address string, port int) *Client {
	return &Client{address: address, port: port, codec: NewMsgCodec()}
}

func (c *Client) conn() error {
	con, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.address, c.port))
	if err != nil {
		return err
	}
	c.con = con
	return nil
}
func (c *Client) Close() error {
	if c.con != nil {
		return c.con.Close()
	}
	return CloseError
}
func (c *Client) Call(method string, argc any) (*Response, error) {
	err := c.conn()
	if err != nil {
		return nil, err
	}
	buff, err := c.codec.Encode(argc)
	if err != nil {
		return nil, err
	}
	err = SendRequest(c.con, NewRequest(method, 0, buff))
	if err != nil {
		return nil, err
	}
	return ReceiveResponse(c.con)
}
