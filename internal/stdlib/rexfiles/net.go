package rexfiles

import (
	"fmt"
	"io"
	"net"

	"github.com/maggisk/rexlang/internal/eval"
)

var NetFFI = map[string]any{
	"tcpListen": eval.MakeBuiltin("tcpListen", func(v eval.Value) (eval.Value, error) {
		port, err := eval.AsInt(v)
		if err != nil {
			return nil, err
		}
		ln, netErr := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if netErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: netErr.Error()}}}, nil
		}
		actualPort := ln.Addr().(*net.TCPAddr).Port
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VTuple{Items: []eval.Value{eval.VListener{L: ln}, eval.VInt{V: actualPort}}}}}, nil
	}),
	"tcpAccept": eval.MakeBuiltin("tcpAccept", func(v eval.Value) (eval.Value, error) {
		ln, ok := v.(eval.VListener)
		if !ok {
			return nil, eval.RuntimeErr("tcpAccept: expected Listener, got %s", eval.ValueToString(v))
		}
		conn, netErr := ln.L.Accept()
		if netErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: netErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VConn{C: conn}}}, nil
	}),
	"tcpConnect": eval.Curried2("tcpConnect", func(hostV, portV eval.Value) (eval.Value, error) {
		host, err := eval.CheckStr("tcpConnect", hostV)
		if err != nil {
			return nil, err
		}
		port, err := eval.AsInt(portV)
		if err != nil {
			return nil, err
		}
		conn, netErr := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		if netErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: netErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VConn{C: conn}}}, nil
	}),
	"tcpRead": eval.MakeBuiltin("tcpRead", func(v eval.Value) (eval.Value, error) {
		c, ok := v.(eval.VConn)
		if !ok {
			return nil, eval.RuntimeErr("tcpRead: expected Conn, got %s", eval.ValueToString(v))
		}
		buf := make([]byte, 4096)
		n, readErr := c.C.Read(buf)
		if readErr != nil {
			if readErr == io.EOF {
				return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: "EOF"}}}, nil
			}
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: readErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VString{V: string(buf[:n])}}}, nil
	}),
	"tcpWrite": eval.Curried2("tcpWrite", func(connV, dataV eval.Value) (eval.Value, error) {
		c, ok := connV.(eval.VConn)
		if !ok {
			return nil, eval.RuntimeErr("tcpWrite: expected Conn, got %s", eval.ValueToString(connV))
		}
		data, err := eval.CheckStr("tcpWrite", dataV)
		if err != nil {
			return nil, err
		}
		_, writeErr := c.C.Write([]byte(data))
		if writeErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: writeErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VUnit{}}}, nil
	}),
	"tcpClose": eval.MakeBuiltin("tcpClose", func(v eval.Value) (eval.Value, error) {
		c, ok := v.(eval.VConn)
		if !ok {
			return nil, eval.RuntimeErr("tcpClose: expected Conn, got %s", eval.ValueToString(v))
		}
		if closeErr := c.C.Close(); closeErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: closeErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VUnit{}}}, nil
	}),
	"tcpCloseListener": eval.MakeBuiltin("tcpCloseListener", func(v eval.Value) (eval.Value, error) {
		ln, ok := v.(eval.VListener)
		if !ok {
			return nil, eval.RuntimeErr("tcpCloseListener: expected Listener, got %s", eval.ValueToString(v))
		}
		if closeErr := ln.L.Close(); closeErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: closeErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VUnit{}}}, nil
	}),
}
