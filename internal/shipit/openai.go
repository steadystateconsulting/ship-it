package shipit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type OpenAIClient struct {
	APIKey string
	Model  string
	Client *http.Client
}

func NewOpenAIClient(model string) (*OpenAIClient, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is required for --planner openai")
	}
	if model == "" {
		model = defaultOpenAIModel()
	}
	return &OpenAIClient{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: 90 * time.Second},
	}, nil
}

func defaultOpenAIModel() string {
	if model := strings.TrimSpace(os.Getenv("SHIPIT_OPENAI_MODEL")); model != "" {
		return model
	}
	return "gpt-5-mini"
}

func (c *OpenAIClient) CreateText(instructions, input string) (string, string, error) {
	body := map[string]any{
		"model": c.Model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{"type": "input_text", "text": input},
				},
			},
		},
		"instructions":      instructions,
		"max_output_tokens": 3000,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(data))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	responseID, _ := payload["id"].(string)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", responseID, fmt.Errorf("openai responses API returned %s: %s", resp.Status, openAIErrorMessage(payload))
	}
	text := extractResponseText(payload)
	if strings.TrimSpace(text) == "" {
		return "", responseID, errors.New("openai response did not contain output text")
	}
	return text, responseID, nil
}

func openAIErrorMessage(payload map[string]any) string {
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		data, _ := json.Marshal(payload)
		return string(data)
	}
	message, _ := errObj["message"].(string)
	if message == "" {
		data, _ := json.Marshal(errObj)
		return string(data)
	}
	return message
}

func extractResponseText(payload map[string]any) string {
	if text, ok := payload["output_text"].(string); ok && text != "" {
		return text
	}
	var b strings.Builder
	output, _ := payload["output"].([]any)
	for _, item := range output {
		itemMap, _ := item.(map[string]any)
		content, _ := itemMap["content"].([]any)
		for _, part := range content {
			partMap, _ := part.(map[string]any)
			if text, ok := partMap["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}
