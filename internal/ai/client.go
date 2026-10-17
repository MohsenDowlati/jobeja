package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

type Client struct {
	client *openai.Client
}

type ParsedTask struct {
	IsTask      bool   `json:"is_task"`
	Message     string `json:"message"`
	ScheduledAt string `json:"scheduled_at"`
	IsRecurring bool   `json:"is_recurring"`
	CronExpr    string `json:"cron_expr"`
	ErrorReason string `json:"error_reason"`
}

func NewClient(apiKey string) *Client {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL("https://api.gapgpt.app/v1"),
	)
	return &Client{client: &client}
}

func (c *Client) TranscribeAudio(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	params := openai.AudioTranscriptionNewParams{
		File:           file,
		Model:          openai.AudioModelWhisper1,
		ResponseFormat: openai.AudioResponseFormatJSON,
	}

	resp, err := c.client.Audio.Transcriptions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("whisper transcription failed: %w", err)
	}

	return resp.Text, nil
}

func (c *Client) ParseTaskPrompt(ctx context.Context, userText string, userTimezone string, currentTime time.Time) (*ParsedTask, error) {
	systemPrompt := fmt.Sprintf(
		"You are an AI assistant for a Telegram task scheduler bot. Current server time is %s and user timezone is %s. "+
			"Extract the task details from the user's message. If the user wants a reminder, set is_task to true, "+
			"calculate the target scheduled time in UTC based on the current time and user timezone, and fill out the JSON schema.",
		currentTime.Format(time.RFC3339),
		userTimezone,
	)

	schemaParam := openai.ResponseFormatJSONSchemaJSONSchemaParam{
		Name:        "TaskExtraction",
		Description: openai.String("Extract task scheduling details from natural language"),
		Strict:      openai.Bool(true),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"is_task": map[string]any{
					"type":        "boolean",
					"description": "True if the message is a task/reminder request, false otherwise.",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "The message or title of the reminder.",
				},
				"scheduled_at": map[string]any{
					"type":        "string",
					"description": "Exact execution time in ISO 8601 UTC format (YYYY-MM-DDTHH:MM:SSZ).",
				},
				"is_recurring": map[string]any{
					"type":        "boolean",
					"description": "True if the task repeats periodically.",
				},
				"cron_expr": map[string]any{
					"type":        "string",
					"description": "Cron expression if recurring, empty string otherwise.",
				},
				"error_reason": map[string]any{
					"type":        "string",
					"description": "Reason if the request is unclear or missing mandatory time.",
				},
			},
			"required":             []string{"is_task", "message", "scheduled_at", "is_recurring", "cron_expr", "error_reason"},
			"additionalProperties": false,
		},
	}

	chatCompletion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4oMini,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userText),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: schemaParam,
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("openai chat completion failed: %w", err)
	}

	if len(chatCompletion.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no completion choices")
	}
	choice := chatCompletion.Choices[0]
	if choice.Message.Refusal != "" {
		return nil, fmt.Errorf("openai declined task extraction")
	}
	if choice.FinishReason != "stop" {
		return nil, fmt.Errorf("openai task extraction incomplete: %s", choice.FinishReason)
	}
	content := choice.Message.Content

	var result ParsedTask
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse structured ai output: %w", err)
	}

	return &result, nil
}
