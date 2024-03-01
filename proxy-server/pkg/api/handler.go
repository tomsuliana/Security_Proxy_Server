package api

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"time"

	commandinjection "proxy-server/pkg/command-injection"
	"proxy-server/pkg/repository"

	"github.com/gorilla/mux"
)

type Handler struct {
	requests  repository.RequestSaver
	responses repository.ResponseSaver
	client    *http.Client
}

const DefaultTimeout = time.Second * 10

func NewHandler(req repository.RequestSaver, resp repository.ResponseSaver) (*Handler, error) {
	transport, err := getTlsTransport()
	if err != nil {
		return nil, err
	}

	return &Handler{
		requests:  req,
		responses: resp,
		client: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: transport,
			Timeout:   DefaultTimeout,
		},
	}, nil
}

func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	req, err := h.requests.Get(mux.Vars(r)["id"])
	if err != nil {
		HttpError(err, w)
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err = encoder.Encode(req)
	if err != nil {
		HttpError(err, w)
		return
	}
}

const kDefaultListSize = 5

func (h *Handler) ListRequests(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if err != nil {
		limit = kDefaultListSize
	}

	requests, err := h.requests.List(limit)
	if err != nil {
		HttpError(err, w)
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err = encoder.Encode(requests)
	if err != nil {
		HttpError(err, w)
		return
	}
}

func (h *Handler) RepeatRequest(w http.ResponseWriter, r *http.Request) {
	req, err := h.requests.GetEncoded(mux.Vars(r)["id"])
	if err != nil {
		HttpError(errors.New("Error getting request: "+err.Error()), w)
		return
	}

	resp, err := h.client.Do(req)
	if err != nil {
		HttpError(errors.New("Error resending request: "+err.Error()), w)
		return
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		HttpError(errors.New("Error resending request: "+err.Error()), w)
		return
	}

	for key, values := range resp.Header {
		for _, elem := range values {
			w.Header().Add(key, elem)
		}
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(bytes)
}

func (h *Handler) DumpRequest(w http.ResponseWriter, r *http.Request) {
	req, err := h.requests.GetEncoded(mux.Vars(r)["id"])
	if err != nil {
		HttpError(err, w)
		return
	}

	bytes, err := httputil.DumpRequest(req, true)
	if err != nil {
		HttpError(err, w)
		return
	}

	w.Write(bytes)
}

func (h *Handler) ScanRequest(w http.ResponseWriter, r *http.Request) {
	const semicolonString = ";cat /etc/passwd;"
	const pipelineString = "|cat /etc/passwd|"
	const appostrofString = "`cat /etc/passwd`"

	req, err := h.requests.GetEncoded(mux.Vars(r)["id"])
	if err != nil {
		HttpError(err, w)
		return
	}

	trySemicolonInjReq, err := commandinjection.TryInjection(req, semicolonString)
	if err != nil {
		HttpError(err, w)
		return
	}

	resp, err := h.client.Do(trySemicolonInjReq)
	if err != nil {
		HttpError(errors.New("Error resending request: "+err.Error()), w)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		HttpError(errors.New("Error reading responce: "+err.Error()), w)
		return
	}

	isVulnarable := false

	if commandinjection.HasRoot(body) {
		isVulnarable = true
	}

	tryPipelineInjReq, err := commandinjection.TryInjection(req, pipelineString)
	if err != nil {
		HttpError(err, w)
		return
	}

	resp, err = h.client.Do(tryPipelineInjReq)
	if err != nil {
		HttpError(errors.New("Error resending request: "+err.Error()), w)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		HttpError(errors.New("Error reading responce: "+err.Error()), w)
		return
	}

	if commandinjection.HasRoot(body) {
		isVulnarable = true
	}

	tryAppostrofInjReq, err := commandinjection.TryInjection(req, appostrofString)
	if err != nil {
		HttpError(err, w)
		return
	}

	resp, err = h.client.Do(tryAppostrofInjReq)
	if err != nil {
		HttpError(errors.New("Error resending request: "+err.Error()), w)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		HttpError(errors.New("Error reading responce: "+err.Error()), w)
		return
	}

	if commandinjection.HasRoot(body) {
		isVulnarable = true
	}

	if isVulnarable {
		w.Write([]byte("Request is vulnarable"))
	} else {
		w.Write([]byte("Request is not vulnarable"))
	}

}

func (h *Handler) GetResponse(w http.ResponseWriter, r *http.Request) {
	resp, err := h.responses.Get(mux.Vars(r)["id"])
	if err != nil {
		HttpError(err, w)
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err = encoder.Encode(resp)
	if err != nil {
		HttpError(err, w)
		return
	}
}

func (h *Handler) GetRequestResponse(w http.ResponseWriter, r *http.Request) {
	resp, err := h.responses.GetByRequest(mux.Vars(r)["id"])
	if err != nil {
		HttpError(err, w)
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err = encoder.Encode(resp)
	if err != nil {
		HttpError(err, w)
		return
	}
}

func (h *Handler) ListResponses(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if err != nil {
		limit = kDefaultListSize
	}

	requests, err := h.responses.List(limit)
	if err != nil {
		HttpError(err, w)
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	err = encoder.Encode(requests)
	if err != nil {
		HttpError(err, w)
		return
	}
}

func getTlsConfig() (*tls.Config, error) {
	cert, err := os.ReadFile("https/ca.crt")
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(cert)
	if !ok {
		return nil, errors.New("error appending certificate")
	}

	return &tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: true,
	}, nil
}

func getTlsTransport() (*http.Transport, error) {
	cfg, err := getTlsConfig()
	if err != nil {
		return nil, err
	}

	return &http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
		TLSClientConfig:     cfg,
	}, nil
}

func WriteError(err error, conn net.Conn) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("Proxy error:" + err.Error())),
	}

	WriteResponse(resp, conn)
}

func HttpError(err error, w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}

func PrintRequest(r *http.Request) {
	bytes, _ := httputil.DumpRequest(r, true)
	fmt.Println(string(bytes))
}

func PrintResponse(r *http.Response, body bool) {
	bytes, _ := httputil.DumpResponse(r, body)
	fmt.Println(string(bytes))
}

func WriteResponse(resp *http.Response, conn net.Conn) error {
	bytes, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return err
	}

	_, err = conn.Write(bytes)
	return err
}
