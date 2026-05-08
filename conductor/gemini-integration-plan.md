# Gemini AI Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable the LeagueWatcher bot to answer questions using Gemini AI when mentioned at the start of a message, with hot-reloadable remote configuration.

**Architecture:** 
1. Enhance configuration to support Gemini settings from Doppler.
2. Inject `ConfigManager` into `Bot` to allow fetching the latest configuration per request.
3. Implement a new `ask` handler that calls the Gemini API via standard `net/http`.
4. Update message routing to detect bot mentions at the beginning of messages.

**Tech Stack:** Go, Discordgo, Google AI Studio (Gemini) API, Doppler.

---

### Task 1: Update Configuration Management

**Files:**
- Modify: `internal/leaguewatcher/config.go`

- [ ] **Step 1: Update `Config` struct**
Add `GeminiAPIKey`, `GeminiSystemPrompt`, and `GeminiModel` to `leaguewatcher.Config`.

```go
type Config struct {
	PollPeriod time.Duration `yaml:"poll_period"`
	PlayedGap  time.Duration `yaml:"played_gap"`

	Players           []Player `yaml:"players"`
	ChannelID         string   `yaml:"channel_id"`
	KhaleesiThreshold *int     `yaml:"khaleesi_threshold,omitempty"`

	// Secrets from Doppler (never logged)
	DiscordToken string `yaml:"-"` // BOT_DISCORD_TOKEN
	OwnerID      string `yaml:"-"` // BOT_OWNER_ID
	GeminiAPIKey string `yaml:"-"` // GEMINI_API_KEY
	GeminiSystemPrompt string `yaml:"-"` // GEMINI_SYSTEM_PROMPT
	GeminiModel  string `yaml:"-"` // GEMINI_MODEL
}
```

- [ ] **Step 2: Update `Reload` method**
Fetch the new secrets in `internal/leaguewatcher/config.go`.

```go
	// Parse gemini_api_key
	if geminiAPIKeySecret, ok := secrets["GEMINI_API_KEY"]; ok && geminiAPIKeySecret.Computed != nil {
		newConfig.GeminiAPIKey = *geminiAPIKeySecret.Computed
	}

	// Parse gemini_system_prompt
	if geminiSystemPromptSecret, ok := secrets["GEMINI_SYSTEM_PROMPT"]; ok && geminiSystemPromptSecret.Computed != nil {
		newConfig.GeminiSystemPrompt = *geminiSystemPromptSecret.Computed
	}

	// Parse gemini_model
	if geminiModelSecret, ok := secrets["GEMINI_MODEL"]; ok && geminiModelSecret.Computed != nil {
		newConfig.GeminiModel = *geminiModelSecret.Computed
	}
	if newConfig.GeminiModel == "" {
		newConfig.GeminiModel = "gemini-2.0-flash"
	}
```

- [ ] **Step 3: Update `LogValue` for security**
Redact the new API key in `LogValue`.

```go
	return slog.GroupValue(
		slog.Duration("poll_period", cfg.PollPeriod),
		slog.Duration("played_gap", cfg.PlayedGap),
		slog.Int("num_players", len(cfg.Players)),
		slog.String("channel_id", cfg.ChannelID),
		slog.Any("khaleesi_threshold", cfg.KhaleesiThreshold),
		slog.String("discord_token", "***REDACTED***"),
		slog.String("owner_id", "***REDACTED***"),
		slog.String("gemini_api_key", "***REDACTED***"),
		slog.String("gemini_model", cfg.GeminiModel),
	)
```

- [ ] **Step 4: Commit**
```bash
git add internal/leaguewatcher/config.go
git commit -m "feat: add Gemini configuration fields to ConfigManager"
```

### Task 2: Inject ConfigManager into Bot

**Files:**
- Modify: `internal/leaguewatcher/bot/bot.go`
- Modify: `cmd/leaguewatcher/main.go`

- [ ] **Step 1: Update `Bot` struct**
Add `configMgr *leaguewatcher.ConfigManager` to `Bot` in `internal/leaguewatcher/bot/bot.go`.

- [ ] **Step 2: Update `bot.New`**
Change `New` signature to accept `*leaguewatcher.ConfigManager`.

```go
func New(cfg Config, configMgr *leaguewatcher.ConfigManager, matchesCh chan leaguewatcher.Match, logger *slog.Logger) (*Bot, error) {
    // ...
    bot := Bot{
        cfg:    cfg,
        configMgr: configMgr,
        // ...
    }
}
```

- [ ] **Step 3: Update `main.go`**
Pass the `configMgr` to `bot.New`.

- [ ] **Step 4: Commit**
```bash
git add internal/leaguewatcher/bot/bot.go cmd/leaguewatcher/main.go
git commit -m "refactor: inject ConfigManager into Bot for hot-reloading"
```

### Task 3: Implement Gemini Handler

**Files:**
- Create: `internal/leaguewatcher/bot/ask.go`

- [ ] **Step 1: Implement `ask` method**
Create `internal/leaguewatcher/bot/ask.go` and implement the `ask` method.

```go
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
```

- [ ] **Step 2: Commit**
```bash
git add internal/leaguewatcher/bot/ask.go
git commit -m "feat: implement Gemini API client and ask handler"
```

### Task 4: Update Routing to Detect Mentions

**Files:**
- Modify: `internal/leaguewatcher/bot/bot.go`

- [ ] **Step 1: Update `cmd` method**
In `internal/leaguewatcher/bot/bot.go`, detect if the message starts with a bot mention.

```go
func (b *Bot) cmd(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	content := strings.TrimSpace(m.Content)
	mention := fmt.Sprintf("<@%s>", s.State.User.ID)
	mentionNick := fmt.Sprintf("<@!%s>", s.State.User.ID)

	isMentioned := false
	query := ""
	if strings.HasPrefix(content, mention) {
		isMentioned = true
		query = strings.TrimSpace(strings.TrimPrefix(content, mention))
	} else if strings.HasPrefix(content, mentionNick) {
		isMentioned = true
		query = strings.TrimSpace(strings.TrimPrefix(content, mentionNick))
	}

	cmd := m.Content
	switch {
	case strings.EqualFold(cmd, "!info"):
		b.info(ctx, s, m)
	// ... (other cases)
	default:
		if isMentioned && query != "" {
			b.ask(ctx, s, m, query)
		} else {
			b.khaleesi(ctx, s, m)
		}
	}
    // ...
}
```

- [ ] **Step 2: Commit**
```bash
git add internal/leaguewatcher/bot/bot.go
git commit -m "feat: route messages starting with bot mention to Gemini ask handler"
```

### Task 5: Integration Testing

**Files:**
- Create: `internal/leaguewatcher/bot/gemini_integration_test.go`

- [ ] **Step 1: Implement Integration Test**
Create `internal/leaguewatcher/bot/gemini_integration_test.go`.

```go
package bot

import (
	"context"
	"leaguewatcher/internal/leaguewatcher"
	"log/slog"
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

	// We can test the 'ask' logic by making a real request if we refactor 'ask' 
    // to separate the API call from the Discord logic, OR we just test the API call logic here.
    // For simplicity, let's verify we can reach Gemini.
    
    // (Implementation of direct API call test here)
}
```

- [ ] **Step 2: Run test**
```bash
DOPPLER_TOKEN=$YOUR_TOKEN go test -v internal/leaguewatcher/bot/gemini_integration_test.go
```

- [ ] **Step 3: Commit**
```bash
git add internal/leaguewatcher/bot/gemini_integration_test.go
git commit -m "test: add Gemini integration test"
```
