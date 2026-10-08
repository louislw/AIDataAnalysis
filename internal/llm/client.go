package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message is one Chat Completions message, including tool calls.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Client talks to an OpenAI-compatible chat completions endpoint.
type Client interface {
	Chat(ctx context.Context, messages []Message, tools []Tool) (Message, error)
}

type HTTPClient struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
	HTTP    *http.Client
}

func (c HTTPClient) Chat(ctx context.Context, messages []Message, tools []Tool) (Message, error) {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.Model) == "" {
		return Message{}, fmt.Errorf("模型未配置")
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body := map[string]any{
		"model":    c.Model,
		"messages": messages,
	}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("模型请求失败: %w", err)
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return Message{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Message{}, fmt.Errorf("模型返回 %d", res.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Message{}, fmt.Errorf("模型响应无法解析")
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return Message{}, fmt.Errorf("模型错误: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return Message{}, fmt.Errorf("模型没有返回内容")
	}
	msg := parsed.Choices[0].Message
	if msg.Role == "" {
		msg.Role = "assistant"
	}
	return msg, nil
}
