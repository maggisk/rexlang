//go:build ignore

package main

import (
	"fmt"
	"io"
	"net"
)

func Stdlib_Net_tcpListen(port int64) (any, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}
	actualPort := int64(ln.Addr().(*net.TCPAddr).Port)
	return Tuple2{F0: ln, F1: actualPort}, nil
}

func Stdlib_Net_tcpAccept(listener any) (any, error) {
	ln := listener.(net.Listener)
	conn, err := ln.Accept()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func Stdlib_Net_tcpConnect(host string, port int64) (any, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func Stdlib_Net_tcpRead(conn any) (string, error) {
	c := conn.(net.Conn)
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		if err == io.EOF {
			return "", fmt.Errorf("EOF")
		}
		return "", err
	}
	return string(buf[:n]), nil
}

func Stdlib_Net_tcpWrite(conn any, data string) error {
	c := conn.(net.Conn)
	_, err := c.Write([]byte(data))
	return err
}

func Stdlib_Net_tcpClose(conn any) error {
	return conn.(net.Conn).Close()
}

func Stdlib_Net_tcpCloseListener(listener any) error {
	return listener.(net.Listener).Close()
}
