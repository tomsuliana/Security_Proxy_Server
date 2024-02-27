package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net"
	"proxy-server/proxy-server/pkg/proxy"
	"time"
)

const PROXYPORT = 8080

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connectionString := fmt.Sprintf("mongodb://%s:%s@%s:%d", "root", "example", "mongo", 27017)
	mongoConnection, err := mongo.Connect(ctx, options.Client().ApplyURI(connectionString))
	if err != nil {
		fmt.Println(err)
	}

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
