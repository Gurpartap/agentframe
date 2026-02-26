package clientapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientStartAndGet(t *testing.T) {
	t.Parallel()

	const token = "test-token"
	const expectedRunID = "run-123"
	const responseJSON = `{"run_id":"run-123","status":"completed","step":2,"version":3,"output":"done"}` + "\n"

	var gotAuthorization string
	var gotStartRequest StartRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/start":
			gotAuthorization = r.Header.Get("Authorization")
			if err := json.NewDecoder(r.Body).Decode(&gotStartRequest); err != nil {
				t.Fatalf("decode start request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, responseJSON)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+expectedRunID:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, responseJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, token, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	maxSteps := 4
	started, rawStart, err := client.Start(context.Background(), StartRequest{
		UserPrompt: "create a file",
		MaxSteps:   &maxSteps,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.RunID != expectedRunID {
		t.Fatalf("start run_id mismatch: got=%q want=%q", started.RunID, expectedRunID)
	}
	if gotAuthorization != "Bearer "+token {
		t.Fatalf("start auth mismatch: got=%q want=%q", gotAuthorization, "Bearer "+token)
	}
	if gotStartRequest.UserPrompt != "create a file" {
		t.Fatalf("start user_prompt mismatch: got=%q", gotStartRequest.UserPrompt)
	}
	if gotStartRequest.MaxSteps == nil || *gotStartRequest.MaxSteps != 4 {
		t.Fatalf("start max_steps mismatch: got=%v", gotStartRequest.MaxSteps)
	}
	if string(rawStart) != responseJSON {
		t.Fatalf("start raw response mismatch: got=%q want=%q", string(rawStart), responseJSON)
	}

	queried, rawGet, err := client.Get(context.Background(), expectedRunID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if queried.Status != "completed" {
		t.Fatalf("get status mismatch: got=%q want=%q", queried.Status, "completed")
	}
	if string(rawGet) != responseJSON {
		t.Fatalf("get raw response mismatch: got=%q want=%q", string(rawGet), responseJSON)
	}
}

func TestClientMapsErrorShape(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs/run-1/cancel" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":{"code":"conflict","message":"run version conflict"}}`)
	}))
	defer server.Close()

	client, err := New(server.URL, "token", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, _, err = client.Cancel(context.Background(), "run-1")
	if err == nil {
		t.Fatalf("expected request error")
	}

	var requestError *RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("expected RequestError, got %T (%v)", err, err)
	}
	if requestError.StatusCode != http.StatusConflict {
		t.Fatalf("status mismatch: got=%d want=%d", requestError.StatusCode, http.StatusConflict)
	}
	if requestError.Code != "conflict" {
		t.Fatalf("error code mismatch: got=%q want=%q", requestError.Code, "conflict")
	}
	if requestError.Message != "run version conflict" {
		t.Fatalf("error message mismatch: got=%q want=%q", requestError.Message, "run version conflict")
	}
}

func TestClientDecodeFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs/start" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{not-json")
	}))
	defer server.Close()

	client, err := New(server.URL, "token", server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, _, err = client.Start(context.Background(), StartRequest{UserPrompt: "hello"})
	if !errors.Is(err, ErrDecodeResponse) {
		t.Fatalf("expected ErrDecodeResponse, got %v", err)
	}
}

func TestClientRequiresAuthTokenForMutatingRoutes(t *testing.T) {
	t.Parallel()

	client, err := New("http://127.0.0.1:8080", "", http.DefaultClient)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, _, err = client.Start(context.Background(), StartRequest{UserPrompt: "hello"})
	if !errors.Is(err, ErrAuthTokenMissing) {
		t.Fatalf("expected ErrAuthTokenMissing, got %v", err)
	}
}

func TestClientRunIDValidation(t *testing.T) {
	t.Parallel()

	client, err := New("http://127.0.0.1:8080", "token", http.DefaultClient)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, _, err = client.Get(context.Background(), "   ")
	if !errors.Is(err, ErrRunIDRequired) {
		t.Fatalf("expected ErrRunIDRequired, got %v", err)
	}
}

func TestMapRequestErrorPlainBodyFallback(t *testing.T) {
	t.Parallel()

	err := mapRequestError(http.StatusBadGateway, []byte("bad upstream"))

	var requestError *RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("expected RequestError, got %T", err)
	}
	if requestError.Code != "" {
		t.Fatalf("expected empty code, got %q", requestError.Code)
	}
	if requestError.Message != "bad upstream" {
		t.Fatalf("message mismatch: got=%q", requestError.Message)
	}
}

func TestDecodeAPIError(t *testing.T) {
	t.Parallel()

	var parsed ErrorResponse
	if ok := decodeAPIError([]byte(`{"error":{"code":"invalid_request","message":"bad input"}}`), &parsed); !ok {
		t.Fatalf("expected decode success")
	}
	if parsed.Error.Code != "invalid_request" {
		t.Fatalf("code mismatch: got=%q", parsed.Error.Code)
	}
	if ok := decodeAPIError([]byte(`{"error":{"message":"missing code"}}`), &parsed); ok {
		t.Fatalf("expected decode failure without code")
	}
	if ok := decodeAPIError([]byte(strings.Repeat("{", 2)), &parsed); ok {
		t.Fatalf("expected decode failure for invalid JSON")
	}
}
