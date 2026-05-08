# Gemini AI Integration Spec

## Overview
Enable the LeagueWatcher bot to answer questions using Google AI Studio (Gemini) when users mention the bot in Discord. All configuration must support hot-reloading from remote config (Doppler).

## Requirements
- Trigger: The message **must begin with a bot mention** (e.g., `<@ID>` or `<@!ID>`).
- Input: The text of the message (stripping the leading mention).
- Processing: Make a call to Google AI Studio's `generateContent` API using Go's standard `net/http` package.
- Prompting: Use a **system prompt loaded from remote configuration** (Doppler).
- Output: Send the AI's response back to the Discord channel as a reply.
- Configuration: `GEMINI_API_KEY`, `GEMINI_SYSTEM_PROMPT`, and `GEMINI_MODEL` must be loaded securely via Doppler and support hot-reload.

## Architecture & Components

### 1. Configuration (`internal/leaguewatcher/config.go`)
- Add the following to the `leaguewatcher.Config` struct:
    - `GeminiAPIKey string`
    - `GeminiSystemPrompt string`
    - `GeminiModel string`
- In `Reload`, read the secrets from Doppler:
    - `secrets["GEMINI_API_KEY"]`
    - `secrets["GEMINI_SYSTEM_PROMPT"]`
    - `secrets["GEMINI_MODEL"]`
- Validation: The bot should skip the Gemini feature if `GEMINI_API_KEY` is not configured.
- Default values: If `GEMINI_MODEL` is empty, default to `gemini-2.0-flash`.

### 2. Bot Architecture (`internal/leaguewatcher/bot/bot.go`)
- To support **hot-reload**, the `Bot` struct needs access to the current configuration for each request.
- Update `Bot` to hold a reference to `ConfigManager` (or update its internal `Config` dynamically).
- Recommendation: Modify `bot.New` to accept the `ConfigManager` or update `main.go` to pass it.

### 3. Command Routing (`internal/leaguewatcher/bot/bot.go`)
- In `cmd(ctx, s, m)`, check if `m.Content` **starts with** the bot's mention (handle both `<@ID>` and `<@!ID>` formats).
- If it starts with the mention and doesn't match existing `!` commands, route to the new `ask(ctx, s, m)` handler.
- Mentions in the middle or end of a message should be ignored for this feature.

### 4. Gemini Client & Handler (`internal/leaguewatcher/bot/ask.go`)
- **New File**: `internal/leaguewatcher/bot/ask.go`
- **`ask(ctx, s, m)` method**:
  - Fetch the **latest** `GeminiSystemPrompt` and `GeminiModel` from the configuration manager.
  - Strip the leading mention from `m.Content` to extract the user's query.
  - Construct the JSON payload for Gemini API:
    ```json
    {
      "system_instruction": {
        "parts": { "text": "LATEST_PROMPT_FROM_CONFIG" }
      },
      "contents": [{
        "parts": [{ "text": "USER_QUERY" }]
      }]
    }
    ```
  - Use `net/http` to send a POST request to `https://generativelanguage.googleapis.com/v1beta/models/{LATEST_MODEL_FROM_CONFIG}:generateContent?key=GEMINI_API_KEY`.
  - Parse the JSON response to extract the content.
  - Send the content back to Discord using `s.ChannelMessageSendReply(m.ChannelID, content, m.Reference())`.
  - Handle errors (API failure, timeout, parsing errors) gracefully.

## Integration Testing

### Goal
Create a real-world integration test to verify the Gemini integration end-to-end.

### Requirements
- **Test File**: `internal/leaguewatcher/bot/gemini_integration_test.go`
- **Setup**:
    - The test should require a `DOPPLER_TOKEN` environment variable.
    - If `DOPPLER_TOKEN` is missing, the test should be skipped (`t.Skip`).
- **Flow**:
    1. Initialize a `ConfigManager` with the `DOPPLER_TOKEN`.
    2. Call `Reload` to fetch real secrets from Doppler.
    3. Verify that `GeminiAPIKey` is present.
    4. Call the internal Gemini request logic with a sample query (e.g., "Hello, who are you?").
- **Verification**:
    - Check that the HTTP request succeeds.
    - Check that the response body is correctly parsed.
    - Check that the resulting answer string is **non-empty**.

## Error Handling
- Timeout: Use a context with a timeout (e.g., 15 seconds) for the HTTP call.
- Missing API Key: If `GEMINI_API_KEY` is missing at request time, return a friendly "I'm not configured for that" message.
- Discord reply failure: Log any errors sending the reply.
