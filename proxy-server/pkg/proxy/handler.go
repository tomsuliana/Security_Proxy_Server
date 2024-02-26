package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

type Handler struct {
	mutex sync.Mutex
}

func NewHandler() (*Handler, error) {
	return &Handler{}, nil
}

func (h *Handler) Handle(connection net.Conn) error {
	req, err := http.ReadRequest(bufio.NewReader(connection))
	if err != nil {
		return err
	}

	return h.handleRequest(connection, req)
}

func (h *Handler) handleRequest(clientConnection net.Conn, toProxy *http.Request) error {
	var hostConnection net.Conn
	var err error
	host := toProxy.URL.Hostname()
	port := getPort(toProxy.URL)

	fmt.Println("host: ", host)
	fmt.Println("port: ", port)
	fmt.Println(toProxy.Method)

	if toProxy.Method == http.MethodConnect {
		fmt.Println("connect")
	}

	toProxy.URL.Scheme = "http"
	hostConnection, err = tcpConnect(host, port)
	if err != nil {
		return err
	}

	defer hostConnection.Close()

	toProxy.URL.Host = ""
	toProxy.URL.Scheme = ""
	toProxy.RequestURI = ""
	toProxy.Header.Del("Proxy-Connection")
	toProxy.Header.Del("Accept-Encoding")
	toProxy.Header.Set("Host", host)

	fmt.Println(toProxy)

	responce, err := sendRequest(hostConnection, toProxy)
	if err != nil {
		return err
	}

	defer responce.Body.Close()

	return writeResponce(responce, clientConnection)

}

func getPort(url *url.URL) string {
	port := url.Port()

	if port == "" {
		if url.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	return port

}

const DefaultTimeout = time.Second * 10

func tcpConnect(host, port string) (net.Conn, error) {
	fmt.Println("tcp", host+":"+port)
	return net.DialTimeout("tcp", host+":"+port, DefaultTimeout)
}

func sendRequest(connection net.Conn, req *http.Request) (*http.Response, error) {
	bytes, err := httputil.DumpRequest(req, true)
	if err != nil {
		return nil, err
	}

	fmt.Println(string(bytes))

	_, err = connection.Write(bytes)
	if err != nil {
		return nil, err
	}

	return http.ReadResponse(bufio.NewReader(connection), req)
}

func writeResponce(resp *http.Response, connection net.Conn) error {
	bytes, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return err
	}
	_, err = connection.Write(bytes)
	return err
}
