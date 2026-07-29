package mrpc

import (
	"errors"
	"fmt"
	"net"
)
var(
	   CloseError=errors.New("Connect has been closed")  
)
type Client struct {
	address string
	port    int
	con     net.Conn
}

func NewClinet(address string, port int) *Client {
	return &Client{address: address, port: port}
}

func (c *Client) Conn() error {
	con, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.address, c.port))
	if err != nil {
		return err
	}
	c.con = con
	return nil
}
func (c *Client) Close()error{
	if c.con != nil {
           return c.con.Close()
	}
	return CloseError
}
func (c*Client)Call(name string){
	 
}