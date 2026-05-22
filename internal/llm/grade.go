package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// TranslationGrade is what Claude returns for the `submit_grade` tool.
type TranslationGrade struct {
	Grade    int    `json:"grade"`
	Feedback string `json:"feedback"`
}

const gradeSystemPrompt = `Ти си искусан учитељ српског језика који оцењује преводе студента (матерњи језик: руски, циљни ниво: C1). Будиш фер, прецизан и кратак.

Оцени корисников превод на скали 0–5:
- 5: савршен — потпуно тачан и природан превод
- 4: добар — мала граматичка или лексичка грешка, али се добро разуме
- 3: прихватљив — значење се преноси, али са приметним грешкама
- 2: делимично тачан — половина значења је тачна
- 1: лоше — превод значајно одступа
- 0: потпуно нетачан

Користи ИСКЉУЧИВО алат submit_grade за одговор. Не пиши текст ван алата.`

// GradeTranslation asks Claude to score the user's translation against a set
// of reference answers. The system prompt is cache-control'd so repeated
// gradings in the same 5-minute window read cache.
func (c *Client) GradeTranslation(ctx context.Context, sourcePrompt string, expected []string, userAnswer string) (TranslationGrade, Usage, error) {
	expectedList := ""
	for i, e := range expected {
		expectedList += fmt.Sprintf("  %d) %s\n", i+1, e)
	}

	userPrompt := fmt.Sprintf(`Извор:
%s

Прихватљиви преводи:
%s

Корисников превод:
%s

Оцени и објасни на ћирилици.`, sourcePrompt, expectedList, userAnswer)

	tool := anthropic.ToolParam{
		Name:        "submit_grade",
		Description: anthropic.String("Submit a translation grade and explanation."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"grade":    map[string]any{"type": "integer", "minimum": 0, "maximum": 5, "description": "SM-2 scale grade"},
				"feedback": map[string]any{"type": "string", "description": "Short explanation in Cyrillic"},
			},
			Required: []string{"grade", "feedback"},
		},
	}

	resp, err := c.sdk.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{{
			Text:         gradeSystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "submit_grade"},
		},
	})
	if err != nil {
		return TranslationGrade{}, Usage{}, fmt.Errorf("anthropic messages.new: %w", err)
	}

	usage := Usage{
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		CacheRead:    int(resp.Usage.CacheReadInputTokens),
		CacheCreate:  int(resp.Usage.CacheCreationInputTokens),
	}

	for _, block := range resp.Content {
		tu, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || tu.Name != "submit_grade" {
			continue
		}
		var g TranslationGrade
		if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &g); err != nil {
			return TranslationGrade{}, usage, fmt.Errorf("parse submit_grade: %w", err)
		}
		if g.Grade < 0 {
			g.Grade = 0
		}
		if g.Grade > 5 {
			g.Grade = 5
		}
		return g, usage, nil
	}
	return TranslationGrade{}, usage, errors.New("no submit_grade tool_use in response")
}
