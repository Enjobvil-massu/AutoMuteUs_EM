package bot

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDownloadAndBase64EncodeReturnsDataURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("png-data"))
	}))
	defer server.Close()

	got, err := downloadAndBase64Encode(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("downloadAndBase64Encode() error = %v", err)
	}
	if got != "data:image/png;base64,cG5nLWRhdGE=" {
		t.Fatalf("downloadAndBase64Encode() = %q", got)
	}
}

func TestDownloadAndBase64EncodeRejectsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := downloadAndBase64Encode(server.Client(), server.URL); err == nil {
		t.Fatal("expected a non-success HTTP response to return an error")
	}
}

func TestDownloadAndBase64EncodeHandlesTransportFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})}

	if _, err := downloadAndBase64Encode(client, "https://example.invalid/emoji.png"); err == nil {
		t.Fatal("expected a transport failure to return an error")
	}
}

func TestDownloadAndBase64EncodeRejectsOversizedBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxEmojiDownloadBytes)+1))),
			Header:     make(http.Header),
		}, nil
	})}

	if _, err := downloadAndBase64Encode(client, "https://example.invalid/emoji.png"); err == nil {
		t.Fatal("expected an oversized emoji response to return an error")
	}
}

func TestDownloadAndBase64EncodeRejectsNilClient(t *testing.T) {
	if _, err := downloadAndBase64Encode(nil, "https://example.invalid/emoji.png"); err == nil {
		t.Fatal("expected a nil HTTP client to return an error")
	}
}
