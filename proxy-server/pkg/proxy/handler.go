package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Handler struct {
	certificates map[string][]byte
	mutex        sync.Mutex
	key          []byte
}

func NewHandler() (*Handler, error) {
	keyInBytes, err := os.ReadFile("https/cert.key")
	if err != nil {
		return nil, err
	}

	certificates, err := loadCertificates()
	if err != nil {
		return nil, err
	}

	return &Handler{
		certificates: certificates,
		key:          keyInBytes,
	}, nil
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

	// fmt.Println("host: ", host)
	// fmt.Println("port: ", port)
	// fmt.Println(toProxy.Method)

	if toProxy.Method == http.MethodConnect {
		clientConnection, err = h.tlsUpgrade(clientConnection, host)
		if err != nil {
			return err
		}

		toProxy, err = http.ReadRequest(bufio.NewReader(clientConnection))
		if err != nil {
			return err
		}

		toProxy.URL.Scheme = "https"
		hostConnection, err = tlsConnect(host, port)
		if err != nil {
			return err
		}
	} else {
		toProxy.URL.Scheme = "http"
		hostConnection, err = tcpConnect(host, port)
		if err != nil {
			return err
		}
	}

	defer hostConnection.Close()

	toProxy.URL.Host = ""
	//toProxy.URL.Scheme = ""
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

func (h *Handler) tlsUpgrade(clientConnection net.Conn, host string) (net.Conn, error) {
	_, err := clientConnection.Write([]byte("HTTP/1.0 200 Connection Established\n\n"))
	if err != nil {
		return nil, err
	}

	err = h.generateCertificate(host)
	if err != nil {
		return nil, err
	}

	cfg, err := h.getTlsConfig(host)
	if err != nil {
		return nil, err
	}

	tlsConnection := tls.Server(clientConnection, cfg)
	clientConnection.SetReadDeadline(time.Now().Add(DefaultTimeout))

	return tlsConnection, nil

}

func (h *Handler) generateCertificate(host string) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	_, certExists := h.certificates[host]
	if !certExists {
		fmt.Printf("Generating certificate for %s\n", host)

		cmd := exec.Command("./https/gen.sh", host)
		var out strings.Builder
		cmd.Stdout = &out

		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("error generating certificate: %v", err)
		}

		certificate := []byte(out.String())

		cert, err := os.Create(fmt.Sprintf("certs/%s.crt", host))
		if err != nil {
			return fmt.Errorf("error generating certificate: %v", err)
		}

		defer cert.Close()

		var written int64 = 0

		written, err = io.Copy(cert, bytes.NewReader(certificate))
		if err != nil {
			return fmt.Errorf("error generating certificate: %v", err)
		}

		if written == 0 {
			certificate = nil
			return errors.New("no bytes written during certificate creation")
		}

		h.certificates[host] = certificate
	}

	return nil

}

func (h *Handler) getTlsConfig(host string) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(h.certificates[host], h.key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
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

func loadCertificates() (map[string][]byte, error) {
	entries, err := os.ReadDir("certs")
	if err != nil {
		return nil, err
	}

	res := make(map[string][]byte, len(entries))

	for _, entry := range entries {
		host := strings.TrimSuffix(entry.Name(), ".crt")

		res[host], err = os.ReadFile("certs/" + entry.Name())
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func tlsConnect(host, port string) (net.Conn, error) {
	dialer := tls.Dialer{}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", host+":"+port)

	return conn, err
}
