package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"leaguewatcher/internal/leaguewatcher"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestGeminiIntegration(t *testing.T) {
	token := os.Getenv("DOPPLER_TOKEN")
	if token == "" {
		t.Skip("DOPPLER_TOKEN not set, skipping integration test")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cm, err := leaguewatcher.NewConfigManager(token, logger)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cm.Reload(ctx); err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	cfg := cm.Get()
	if cfg.GeminiAPIKey == "" {
		t.Skip("GEMINI_API_KEY not found in Doppler, skipping integration test")
	}

	// Test the API call logic directly
	query := "Сколько боссов в бвл?"
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{{Text: query}},
			},
		},
	}

	if cfg.GeminiSystemPrompt != "" {
		reqBody.SystemInstruction = &geminiSystemInstruction{
			Parts: geminiPart{Text: cfg.GeminiSystemPrompt},
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal gemini request: %v", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", cfg.GeminiModel, cfg.GeminiAPIKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("failed to create http request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("failed to call gemini api: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("gemini api returned error: status=%s, body=%s", resp.Status, string(body))
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		t.Fatalf("failed to decode gemini response: %v", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		t.Fatal("gemini returned no candidates or parts")
	}

	answer := geminiResp.Candidates[0].Content.Parts[0].Text
	if answer == "" {
		t.Fatal("gemini returned an empty answer")
	}

	t.Logf("Gemini response: %s", answer)
}
