package testsupport

import (
	"net/http/httptest"
	"testing"

	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/httpapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

func NewMockHTTPServer(t testing.TB, authToken string) *httptest.Server {
	t.Helper()

	cfg := config.Default()
	cfg.ModelMode = config.ModelModeMock
	cfg.ToolMode = config.ToolModeMock

	runtime, err := runtimewire.New(cfg)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	policy := httpapi.DefaultPolicyConfig()
	if authToken != "" {
		policy.AuthToken = authToken
	}

	return httptest.NewServer(httpapi.NewRouter(runtime, policy))
}
