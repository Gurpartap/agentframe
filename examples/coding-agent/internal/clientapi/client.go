package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

func New(baseURL, authToken string, httpClient *http.Client) (*Client, error) {
	trimmedBaseURL := strings.TrimSpace(baseURL)
	if trimmedBaseURL == "" {
		return nil, fmt.Errorf("new client: base URL is required")
	}

	parsed, err := url.Parse(trimmedBaseURL)
	if err != nil {
		return nil, fmt.Errorf("new client: parse base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("new client: base URL must include scheme and host")
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    strings.TrimRight(trimmedBaseURL, "/"),
		authToken:  strings.TrimSpace(authToken),
		httpClient: httpClient,
	}, nil
}

func (c *Client) Health(ctx context.Context) ([]byte, error) {
	return c.doRaw(ctx, http.MethodGet, "/healthz", nil, false)
}

func (c *Client) Start(ctx context.Context, request StartRequest) (RunState, []byte, error) {
	var response RunState
	raw, err := c.doJSON(ctx, http.MethodPost, "/v1/runs/start", request, &response, true)
	if err != nil {
		return RunState{}, nil, err
	}
	return response, raw, nil
}

func (c *Client) Get(ctx context.Context, runID string) (RunState, []byte, error) {
	path, err := runPath(runID)
	if err != nil {
		return RunState{}, nil, err
	}

	var response RunState
	raw, err := c.doJSON(ctx, http.MethodGet, path, nil, &response, false)
	if err != nil {
		return RunState{}, nil, err
	}
	return response, raw, nil
}

func (c *Client) Continue(ctx context.Context, runID string, request ContinueRequest) (RunState, []byte, error) {
	path, err := runPath(runID)
	if err != nil {
		return RunState{}, nil, err
	}

	var response RunState
	raw, err := c.doJSON(ctx, http.MethodPost, path+"/continue", request, &response, true)
	if err != nil {
		return RunState{}, nil, err
	}
	return response, raw, nil
}

func (c *Client) Steer(ctx context.Context, runID string, request SteerRequest) (RunState, []byte, error) {
	path, err := runPath(runID)
	if err != nil {
		return RunState{}, nil, err
	}

	var response RunState
	raw, err := c.doJSON(ctx, http.MethodPost, path+"/steer", request, &response, true)
	if err != nil {
		return RunState{}, nil, err
	}
	return response, raw, nil
}

func (c *Client) FollowUp(ctx context.Context, runID string, request FollowUpRequest) (RunState, []byte, error) {
	path, err := runPath(runID)
	if err != nil {
		return RunState{}, nil, err
	}

	var response RunState
	raw, err := c.doJSON(ctx, http.MethodPost, path+"/follow-up", request, &response, true)
	if err != nil {
		return RunState{}, nil, err
	}
	return response, raw, nil
}

func (c *Client) Cancel(ctx context.Context, runID string) (RunState, []byte, error) {
	path, err := runPath(runID)
	if err != nil {
		return RunState{}, nil, err
	}

	var response RunState
	raw, err := c.doJSON(ctx, http.MethodPost, path+"/cancel", nil, &response, true)
	if err != nil {
		return RunState{}, nil, err
	}
	return response, raw, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any, requiresAuth bool) ([]byte, error) {
	raw, err := c.doRaw(ctx, method, path, payload, requiresAuth)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return raw, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecodeResponse, err)
	}
	return raw, nil
}

func (c *Client) doRaw(ctx context.Context, method, path string, payload any, requiresAuth bool) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var bodyReader io.Reader
	if payload != nil {
		var encoded bytes.Buffer
		if err := json.NewEncoder(&encoded).Encode(payload); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrEncodeRequest, err)
		}
		bodyReader = &encoded
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if requiresAuth {
		if c.authToken == "" {
			return nil, ErrAuthTokenMissing
		}
		request.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadResponse, err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, mapRequestError(response.StatusCode, body)
	}
	return body, nil
}

func decodeAPIError(body []byte, out *ErrorResponse) bool {
	if len(body) == 0 || out == nil {
		return false
	}
	*out = ErrorResponse{}
	if err := json.Unmarshal(body, out); err != nil {
		return false
	}
	return strings.TrimSpace(out.Error.Code) != ""
}

func runPath(runID string) (string, error) {
	trimmedRunID := strings.TrimSpace(runID)
	if trimmedRunID == "" {
		return "", ErrRunIDRequired
	}
	return "/v1/runs/" + url.PathEscape(trimmedRunID), nil
}
