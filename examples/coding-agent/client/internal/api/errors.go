package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrReadResponse     = errors.New("read response")
	ErrEncodeRequest    = errors.New("encode request")
	ErrDecodeResponse   = errors.New("decode response")
	ErrAuthTokenMissing = errors.New("auth token is required for mutating routes")
	ErrRunIDRequired    = errors.New("run id is required")
)

type RequestError struct {
	StatusCode int
	Code       string
	Message    string
	Body       []byte
}

func (e *RequestError) Error() string {
	if e == nil {
		return "<nil>"
	}

	statusText := http.StatusText(e.StatusCode)
	if statusText == "" {
		statusText = "unknown status"
	}

	if e.Code != "" {
		return fmt.Sprintf("request failed: status=%d (%s) code=%s message=%s", e.StatusCode, statusText, e.Code, e.Message)
	}
	return fmt.Sprintf("request failed: status=%d (%s) message=%s", e.StatusCode, statusText, e.Message)
}

func mapRequestError(statusCode int, body []byte) error {
	trimmedBody := strings.TrimSpace(string(body))
	if trimmedBody == "" {
		trimmedBody = http.StatusText(statusCode)
	}

	var parsed ErrorResponse
	if decodeAPIError(body, &parsed) {
		return &RequestError{
			StatusCode: statusCode,
			Code:       parsed.Error.Code,
			Message:    parsed.Error.Message,
			Body:       append([]byte(nil), body...),
		}
	}

	return &RequestError{
		StatusCode: statusCode,
		Message:    trimmedBody,
		Body:       append([]byte(nil), body...),
	}
}
