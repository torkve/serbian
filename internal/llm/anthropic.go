package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Client wraps the Anthropic SDK for the narrow set of calls this app makes:
// pregen task generation, and (later) translation answer grading.
type Client struct {
	sdk   anthropic.Client
	model anthropic.Model
}

func NewClient(apiKey, model string) *Client {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	m := anthropic.ModelClaudeOpus4_7
	if model != "" {
		m = anthropic.Model(model)
	}
	return &Client{sdk: c, model: m}
}

// GeneratedTask is one entry returned by the `submit_tasks` tool.
type GeneratedTask struct {
	Prompt    string          `json:"prompt"`
	Payload   json.RawMessage `json:"payload"`
	Expected  json.RawMessage `json:"expected"`
	Rationale string          `json:"rationale,omitempty"`
}

// Usage from the most recent call (for budget accounting).
type Usage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheCreate  int
}

// GenerateTasks asks Claude to emit `count` structured tasks for `kind`/`topic`
// via the `submit_tasks` tool. The system prompt is cached so subsequent
// calls (same prompt) only pay full price on first use.
func (c *Client) GenerateTasks(ctx context.Context, systemPrompt, userPrompt string, count int) ([]GeneratedTask, Usage, error) {
	tool := anthropic.ToolParam{
		Name:        "submit_tasks",
		Description: anthropic.String("Submit a batch of generated Serbian-learning tasks. Use ONLY this tool to respond."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"tasks": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt":    map[string]any{"type": "string", "description": "User-facing task text, in Cyrillic."},
							"payload":   map[string]any{"type": "object", "description": "Kind-specific structured fields."},
							"expected":  map[string]any{"type": "object", "description": "Acceptable answers and constraints."},
							"rationale": map[string]any{"type": "string", "description": "Short grammar explanation in Cyrillic."},
						},
						"required": []string{"prompt", "payload", "expected"},
					},
					"minItems": count,
					"maxItems": count,
				},
			},
			Required: []string{"tasks"},
		},
	}

	resp, err := c.sdk.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 8192,
		System: []anthropic.TextBlockParam{{
			Text:         systemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "submit_tasks"},
		},
	})
	if err != nil {
		return nil, Usage{}, fmt.Errorf("anthropic messages.new: %w", err)
	}

	usage := Usage{
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		CacheRead:    int(resp.Usage.CacheReadInputTokens),
		CacheCreate:  int(resp.Usage.CacheCreationInputTokens),
	}

	for _, block := range resp.Content {
		tu, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || tu.Name != "submit_tasks" {
			continue
		}
		var input struct {
			Tasks []GeneratedTask `json:"tasks"`
		}
		raw := tu.JSON.Input.Raw()
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, usage, fmt.Errorf("parse submit_tasks input: %w", err)
		}
		return input.Tasks, usage, nil
	}
	return nil, usage, errors.New("response contained no submit_tasks tool_use block")
}
