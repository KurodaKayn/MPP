package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientDoesNotFollowRedirects(t *testing.T) {
	redirectTargetHit := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/user/dashboard/stats":
			http.Redirect(writer, request, "/login", http.StatusFound)
		case "/login":
			redirectTargetHit = true
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("login"))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	response, err := NewHTTPClient(1).Get(server.URL+"/api/user/dashboard/stats", nil)
	if err != nil {
		t.Fatal(err)
	}

	if response.Status != http.StatusFound {
		t.Fatalf("expected redirect response to be preserved, got HTTP %d", response.Status)
	}
	if redirectTargetHit {
		t.Fatal("expected redirect target not to be requested")
	}
	if location := response.Headers.Get("Location"); location != "/login" {
		t.Fatalf("expected redirect Location header to be preserved, got %q", location)
	}
}
