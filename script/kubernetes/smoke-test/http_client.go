package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Response struct {
	Status  int
	Body    string
	Headers http.Header
}

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient(timeoutSeconds int) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

func (client *HTTPClient) Get(targetURL string, headers map[string]string) (Response, error) {
	return client.request(http.MethodGet, targetURL, headers, nil)
}

func (client *HTTPClient) Post(targetURL string, headers map[string]string, jsonBody any) (Response, error) {
	return client.request(http.MethodPost, targetURL, headers, jsonBody)
}

func (client *HTTPClient) Delete(targetURL string, headers map[string]string) (Response, error) {
	return client.request(http.MethodDelete, targetURL, headers, nil)
}

func (client *HTTPClient) request(method string, targetURL string, headers map[string]string, jsonBody any) (Response, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return Response{}, CheckFailure(fmt.Sprintf("invalid URL %q: %s", targetURL, err))
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return Response{}, CheckFailure(fmt.Sprintf("invalid URL %q: missing scheme or host", targetURL))
	}

	var body io.Reader
	if jsonBody != nil {
		payload, err := json.Marshal(jsonBody)
		if err != nil {
			return Response{}, err
		}
		body = bytes.NewReader(payload)
	}

	request, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return Response{}, CheckFailure(fmt.Sprintf("invalid URL %q: %s", targetURL, err))
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if jsonBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.client.Do(request)
	if err != nil {
		return Response{}, httpFailure(targetURL, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return Response{}, httpFailure(targetURL, err)
	}
	return Response{
		Status:  response.StatusCode,
		Body:    string(responseBody),
		Headers: response.Header,
	}, nil
}

func httpFailure(targetURL string, err error) error {
	if netErr, ok := err.(net.Error); ok {
		return CheckFailure(fmt.Sprintf("HTTP request to %s failed: %T: %s", targetURL, netErr, netErr.Error()))
	}
	return CheckFailure(fmt.Sprintf("HTTP request to %s failed: %T: %s", targetURL, err, err.Error()))
}
