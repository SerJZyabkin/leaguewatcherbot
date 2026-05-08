package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bwmarrin/discordgo"
)

type geminiRequest struct {
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	Contents          []geminiContent           `json:"contents"`
}

type geminiSystemInstruction struct {
	Parts geminiPart `json:"parts"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (b *Bot) ask(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, query string) {
	cfg := b.configMgr.Get()
	if cfg.GeminiAPIKey == "" {
		s.ChannelMessageSendReply(m.ChannelID, "I'm not configured for that, sorry!", m.Reference())
		return
	}

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
		b.logger.Error("failed to marshal gemini request", "error", err)
		return
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", cfg.GeminiModel, cfg.GeminiAPIKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		b.logger.Error("failed to create http request", "error", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		b.logger.Error("failed to call gemini api", "error", err)
		s.ChannelMessageSendReply(m.ChannelID, "I had trouble contacting the oracle.", m.Reference())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		b.logger.Error("gemini api returned error", "status", resp.Status, "body", string(body))
		s.ChannelMessageSendReply(m.ChannelID, "The oracle is in a bad mood right now.", m.Reference())
		return
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		b.logger.Error("failed to decode gemini response", "error", err)
		return
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		s.ChannelMessageSendReply(m.ChannelID, "The oracle remains silent.", m.Reference())
		return
	}

	answer := geminiResp.Candidates[0].Content.Parts[0].Text
	if answer == "" {
		s.ChannelMessageSendReply(m.ChannelID, "The oracle's vision is clouded.", m.Reference())
		return
	}

	s.ChannelMessageSendReply(m.ChannelID, answer, m.Reference())
}
