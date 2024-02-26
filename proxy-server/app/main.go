package main

import (
	"fmt"
	"net"
	"proxy-server/proxy-server/pkg/proxy"
)

const PROXYPORT = 8080

func main() {
	proxyHandler, err := proxy.NewHandler()
	if err != nil {
		fmt.Println(err)
		return
	}

	proxyListener, err := net.ListenTCP("tcp", &net.TCPAddr{
		Port: PROXYPORT,
	})

	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Proxy listening at port %d \n", PROXYPORT)

	for {
		connection, err := proxyListener.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}

		go func() {
			defer connection.Close()
			err := proxyHandler.Handle(connection)
			if err != nil {
				fmt.Println(err)
			}
		}()
	}

}
