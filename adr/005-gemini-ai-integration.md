# ADR 005: Gemini AI Integration for Natural Language Interaction

## Status

**Accepted** - 2026-05-08

## Context

The League Watcher Bot currently provides match tracking and some fun features like "Pidor of the Day" and "Khaleesi" style message replacements. However, users want a more interactive experience where they can ask the bot questions directly.

### Problems with Current Approach

- **Static Responses**: Commands like `!info` are hardcoded and don't provide context-aware information.
- **Limited Interaction**: The bot can only respond to specific command prefixes (`!`).
- **No Intelligence**: There is no way to ask general questions or get AI-driven insights about matches or other topics within Discord.

### Requirements for Solution

1. **Natural Language Processing**: Ability to answer open-ended questions.
2. **Discord Integration**: Triggered by mentioning the bot at the start of a message.
3. **Hot Reload**: System prompt and model choice should be configurable via Doppler without restart.
4. **Performance**: Use a fast, efficient model (e.g., Gemini 2.0 Flash) to minimize response latency.
5. **Security**: API keys must be managed via Doppler.
6. **No Heavy Dependencies**: Use standard Go `net/http` client for API calls.

## Decision

We will integrate **Google AI Studio (Gemini API)** to provide natural language capabilities:

### 1. Configuration Enhancements

**Location**: `internal/leaguewatcher/config.go`

**New Fields**:
- `GeminiAPIKey`: The API key for Google AI Studio.
- `GeminiSystemPrompt`: The instruction set that defines the bot's persona.
- `GeminiModel`: The specific Gemini model to use (default: `gemini-2.0-flash`).

**Doppler Mapping**:
| Doppler Secret | Type | Config Field | Default |
|----------------|------|--------------|---------|
| `GEMINI_API_KEY` | String | `GeminiAPIKey` | - |
| `GEMINI_SYSTEM_PROMPT` | String | `GeminiSystemPrompt` | - |
| `GEMINI_MODEL` | String | `GeminiModel` | `gemini-2.0-flash` |

### 2. Bot Architecture Update

To support hot-reload of AI settings, the `Bot` will now hold a reference to the `ConfigManager` instead of a static `Config` snapshot for these fields.

**Changes**:
- Update `internal/leaguewatcher/bot/bot.go` to include `configMgr *leaguewatcher.ConfigManager`.
- Update `bot.New` to accept the `ConfigManager`.
- Modify `cmd/leaguewatcher/main.go` to pass the `ConfigManager` to the bot during initialization.

### 3. Trigger & Routing Logic

The bot will respond if a message **starts with** its mention.

**Logic**:
- Detect mentions in `<@ID>` or `<@!ID>` format at the beginning of `m.Content`.
- If a mention is found and no `!` command matches, route to the `ask` handler.
- Strip the leading mention to extract the user's query.

### 4. Gemini API Client Implementation

**Location**: `internal/leaguewatcher/bot/ask.go`

**Implementation Details**:
- Uses standard `net/http` client with a 15-second timeout.
- Calls the `generateContent` endpoint: `https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={apiKey}`.
- Supports `system_instruction` for persona enforcement.
- Graceful error handling for API failures or missing configuration.

### 5. Integration Testing

A new test `internal/leaguewatcher/bot/gemini_integration_test.go` will be added to verify the end-to-end flow:
- Loads config from Doppler (requires `DOPPLER_TOKEN`).
- Makes a real API call to Gemini.
- Verifies a non-empty response.

## Consequences

### Positive

1. **Rich Interaction**: Users can interact with the bot using natural language.
2. **Dynamic Persona**: The bot's personality can be updated instantly via Doppler.
3. **Low Latency**: Gemini 2.0 Flash provides quick responses suitable for Discord.
4. **Maintainable**: Standard library usage avoids bloat and dependency management issues.
5. **Secure**: API keys are never hardcoded or stored on disk.

### Negative

1. **API Cost**: Large volumes of messages could incur costs (though Gemini has a generous free tier).
2. **Hallucinations**: As with all LLMs, the bot might provide inaccurate information.
3. **External Dependency**: Reliance on Google AI Studio's availability.

### Neutral

1. **Standard Library vs SDK**: We chose `net/http` over the official Gemini Go SDK to keep the binary size small and reduce dependencies, as our use case is simple.

## Alternatives Considered

### Alternative 1: Perplexity AI

**Rejected Because**:
- While great for search, Gemini offers easier integration and a very capable free tier for chat-like interactions.

### Alternative 2: OpenAI (GPT-4o/mini)

**Rejected Because**:
- Gemini 2.0 Flash offers comparable performance with better pricing/free tier limits for this specific scale.

### Alternative 3: Hardcoded System Prompt

**Rejected Because**:
- Doesn't allow for "live" personality updates without redeployment.

## References

- **Gemini API Documentation**: https://ai.google.dev/docs
- **Discordgo Reference**: https://pkg.go.dev/github.com/bwmarrin/discordgo

## Reviewers

- **Author**: Gemini CLI
- **Date**: 2026-05-08
