package policyauth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	HeaderAuthorization = "Authorization"
	BearerPrefix        = "Bearer "
	DefaultToken        = "coding-agent-dev-token"
)

var ErrUnauthorized = errors.New("policy authentication failed")

type RejectFunc func(http.ResponseWriter, *http.Request, error)

func Middleware(token string, reject RejectFunc) func(http.Handler) http.Handler {
	expected := strings.TrimSpace(token)
	if expected == "" {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	expectedHeader := BearerPrefix + expected

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := strings.TrimSpace(r.Header.Get(HeaderAuthorization))
			if provided != expectedHeader {
				reject(w, r, fmt.Errorf("%w: missing or invalid bearer token", ErrUnauthorized))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
