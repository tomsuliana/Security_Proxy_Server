package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"proxy-server/pkg/api"
	"proxy-server/pkg/proxy"
	"proxy-server/pkg/repository"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	requests := repository.NewMongoRequestSaver(mongoConnection)
	responses := repository.NewMongoResponseSaver(mongoConnection)

	proxyHandler, err := proxy.NewHandler(requests, responses)
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

	go startApi(requests, responses)

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

func startApi(req repository.RequestSaver, resp repository.ResponseSaver) {
	router := mux.NewRouter()

	handler, err := api.NewHandler(req, resp)
	if err != nil {
		fmt.Println(err)
		return
	}

	router.HandleFunc("/requests", handler.ListRequests)
	router.HandleFunc("/requests/{id}", handler.GetRequest)
	router.HandleFunc("/repeat/{id}", handler.RepeatRequest)
	router.HandleFunc("/scan/{id}", handler.ScanRequest)
	router.HandleFunc("/requests/{id}/dump", handler.DumpRequest)

	router.HandleFunc("/responses", handler.ListResponses)
	router.HandleFunc("/responses/{id}", handler.GetResponse)
	router.HandleFunc("/requests/{id}/response", handler.GetRequestResponse)

	fmt.Println("Api listening at port 8000...")

	http.ListenAndServe(":8000", router)
}
