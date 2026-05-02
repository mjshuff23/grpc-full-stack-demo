package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Model struct {
	Name      string
	SizeBytes uint64
}

type GenerateRequest struct {
	Model  string
	System string
	Prompt string
}

type Client interface {
	ListModels(context.Context) ([]Model, error)
	Generate(context.Context, GenerateRequest, func(string) error) (string, error)
}

type OllamaClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewOllamaClient(baseURL string, httpClient *http.Client) *OllamaClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OllamaClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *OllamaClient) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama tags request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama tags returned %s: %s", res.Status, readBody(res.Body))
	}

	var payload struct {
		Models []struct {
			Name string `json:"name"`
			Size uint64 `json:"size"`
		} `json:"models"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode ollama tags: %w", err)
	}

	models := make([]Model, 0, len(payload.Models))
	for _, model := range payload.Models {
		models = append(models, Model{Name: model.Name, SizeBytes: model.Size})
	}
	return models, nil
}

func (c *OllamaClient) Generate(ctx context.Context, request GenerateRequest, onToken func(string) error) (string, error) {
	payload := map[string]any{
		"model":  request.Model,
		"stream": true,
		"messages": []map[string]string{
			{"role": "system", "content": request.System},
			{"role": "user", "content": request.Prompt},
		},
		"options": map[string]any{
			"temperature": 0.7,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("ollama chat returned %s: %s", res.Status, readBody(res.Body))
	}

	var builder strings.Builder
	decoder := json.NewDecoder(res.Body)
	for decoder.More() {
		var event struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done  bool   `json:"done"`
			Error string `json:"error"`
		}
		if err := decoder.Decode(&event); err != nil {
			return "", fmt.Errorf("decode ollama chat stream: %w", err)
		}
		if event.Error != "" {
			return "", fmt.Errorf("ollama chat error: %s", event.Error)
		}
		if event.Message.Content != "" {
			builder.WriteString(event.Message.Content)
			if onToken != nil {
				if err := onToken(event.Message.Content); err != nil {
					return "", err
				}
			}
		}
		if event.Done {
			break
		}
	}

	return builder.String(), nil
}

func readBody(reader io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(reader, 4096))
	if err != nil {
		return err.Error()
	}
	return string(body)
}
