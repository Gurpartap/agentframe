package modelopenai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

const (
	defaultBaseURL  = "https://api.openai.com/v1"
	defaultEndpoint = "/chat/completions"
	defaultTimeout  = 30 * time.Second
)

type Config struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

type Adapter struct {
	apiKey      string
	model       string
	endpointURL string
	httpClient  *http.Client
}

var _ agentreact.Model = (*Adapter)(nil)

func New(cfg Config) (*Adapter, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("new model adapter: api key is required")
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("new model adapter: model is required")
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	endpointURL := strings.TrimRight(baseURL, "/") + defaultEndpoint

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &Adapter{
		apiKey:      apiKey,
		model:       model,
		endpointURL: endpointURL,
		httpClient:  httpClient,
	}, nil
}

func (a *Adapter) Generate(ctx context.Context, request agentreact.ModelRequest) (agent.Message, error) {
	requestPayload, err := buildRequest(a.model, request)
	if err != nil {
		return agent.Message{}, fmt.Errorf("provider request: %w", err)
	}

	encoded, err := json.Marshal(requestPayload)
	if err != nil {
		return agent.Message{}, fmt.Errorf("provider request encode: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpointURL, bytes.NewReader(encoded))
	if err != nil {
		return agent.Message{}, fmt.Errorf("provider request build: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := a.httpClient.Do(httpRequest)
	if err != nil {
		return agent.Message{}, fmt.Errorf("provider request execute: %w", err)
	}
	defer response.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return agent.Message{}, fmt.Errorf("provider response read: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.Message{}, fmt.Errorf(
			"provider response status=%d body=%s",
			response.StatusCode,
			string(bodyBytes),
		)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return agent.Message{}, fmt.Errorf("provider response decode: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return agent.Message{}, fmt.Errorf("provider response decode: no choices")
	}

	message, err := toAgentMessage(parsed.Choices[0].Message)
	if err != nil {
		return agent.Message{}, fmt.Errorf("provider response decode: %w", err)
	}
	return message, nil
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
}

type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func buildRequest(model string, request agentreact.ModelRequest) (chatCompletionRequest, error) {
	normalizedMessages, err := normalizeProviderMessages(request.Messages)
	if err != nil {
		return chatCompletionRequest{}, err
	}

	messages := make([]chatMessage, len(normalizedMessages))
	for i := range normalizedMessages {
		converted, err := toChatMessage(normalizedMessages[i])
		if err != nil {
			return chatCompletionRequest{}, err
		}
		messages[i] = converted
	}

	tools := make([]chatTool, len(request.Tools))
	for i := range request.Tools {
		tools[i] = chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name:        request.Tools[i].Name,
				Description: request.Tools[i].Description,
				Parameters:  request.Tools[i].InputSchema,
			},
		}
	}

	return chatCompletionRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
	}, nil
}

func normalizeProviderMessages(messages []agent.Message) ([]agent.Message, error) {
	normalized := make([]agent.Message, 0, len(messages))
	assistantToolCalls := make(map[string]struct{}, len(messages))
	toolMessageIndexByCallID := make(map[string]int, len(messages))

	for i := range messages {
		message := agent.CloneMessage(messages[i])
		switch message.Role {
		case agent.RoleAssistant:
			normalized = append(normalized, message)
			for _, call := range message.ToolCalls {
				if call.ID == "" {
					continue
				}
				assistantToolCalls[call.ID] = struct{}{}
			}
		case agent.RoleTool:
			toolCallID := strings.TrimSpace(message.ToolCallID)
			if toolCallID == "" {
				return nil, fmt.Errorf("decode messages: tool message at index %d missing tool_call_id", i)
			}
			if _, ok := assistantToolCalls[toolCallID]; !ok {
				return nil, fmt.Errorf(
					"decode messages: tool message at index %d references unknown tool_call_id %q",
					i,
					toolCallID,
				)
			}
			if existingIndex, exists := toolMessageIndexByCallID[toolCallID]; exists {
				// Keep only the latest tool observation for a call in provider payloads.
				normalized[existingIndex] = message
			} else {
				normalized = append(normalized, message)
				toolMessageIndexByCallID[toolCallID] = len(normalized) - 1
			}
		default:
			normalized = append(normalized, message)
		}
	}
	return normalized, nil
}

func toChatMessage(message agent.Message) (chatMessage, error) {
	role, err := toProviderRole(message.Role)
	if err != nil {
		return chatMessage{}, err
	}

	toolCalls := make([]chatToolCall, len(message.ToolCalls))
	for i := range message.ToolCalls {
		arguments := "{}"
		if len(message.ToolCalls[i].Arguments) > 0 {
			encoded, err := json.Marshal(message.ToolCalls[i].Arguments)
			if err != nil {
				return chatMessage{}, fmt.Errorf("encode tool call arguments: %w", err)
			}
			arguments = string(encoded)
		}
		toolCalls[i] = chatToolCall{
			ID:   message.ToolCalls[i].ID,
			Type: "function",
			Function: chatToolCallFunction{
				Name:      message.ToolCalls[i].Name,
				Arguments: arguments,
			},
		}
	}

	return chatMessage{
		Role:       role,
		Content:    message.Content,
		Name:       message.Name,
		ToolCallID: message.ToolCallID,
		ToolCalls:  toolCalls,
	}, nil
}

func toProviderRole(role agent.Role) (string, error) {
	switch role {
	case agent.RoleSystem:
		return "system", nil
	case agent.RoleUser:
		return "user", nil
	case agent.RoleAssistant:
		return "assistant", nil
	case agent.RoleTool:
		return "tool", nil
	default:
		return "", fmt.Errorf("unsupported message role %q", role)
	}
}

func toAgentMessage(message chatMessage) (agent.Message, error) {
	if message.Role != "assistant" {
		return agent.Message{}, fmt.Errorf("expected assistant message role, got %q", message.Role)
	}

	toolCalls := make([]agent.ToolCall, len(message.ToolCalls))
	for i := range message.ToolCalls {
		arguments := map[string]any{}
		if strings.TrimSpace(message.ToolCalls[i].Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(message.ToolCalls[i].Function.Arguments), &arguments); err != nil {
				return agent.Message{}, fmt.Errorf(
					"decode tool call arguments for %q: %w",
					message.ToolCalls[i].Function.Name,
					err,
				)
			}
		}
		toolCalls[i] = agent.ToolCall{
			ID:        message.ToolCalls[i].ID,
			Name:      message.ToolCalls[i].Function.Name,
			Arguments: arguments,
		}
	}

	return agent.Message{
		Role:      agent.RoleAssistant,
		Content:   message.Content,
		ToolCalls: toolCalls,
	}, nil
}
