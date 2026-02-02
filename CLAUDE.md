# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build (requires CGO for SQLite FTS5)
CGO_ENABLED=1 go build -tags "fts5" -o dreamteller ./cmd/dreamteller

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test -v ./internal/token/...

# Run the application
./dreamteller --help
./dreamteller open <project-name>
```

## Architecture Overview

Dreamteller is a TUI application for AI-assisted novel writing built with Bubble Tea. It maintains context (characters, settings, plot) that automatically informs AI responses.

### Core Layers

```
cmd/dreamteller/     CLI entry (Cobra commands: new, open, list, reindex)
        ↓
internal/app/        App lifecycle, global/project config management
        ↓
internal/tui/        Bubble Tea TUI (Elm architecture: Model→Update→View)
        ↓
internal/llm/        Provider interface + adapters (OpenAI, Gemini, Local)
        ↓
internal/token/      Token counting (tiktoken) + budget allocation
internal/search/     FTS5 full-text search + chunked indexing
internal/storage/    SQLite + atomic file operations
internal/project/    Project CRUD, directory structure
        ↓
pkg/types/           Shared domain models
```

### Key Patterns

**Provider Adapter Pattern** (`internal/llm/`):
- `Provider` interface with `Chat()`, `Stream()`, `Capabilities()`, `Close()`
- Adapters in `adapters/` translate to provider-specific APIs
- Capabilities negotiation for tools/streaming support

**Token Budget System** (`internal/token/budget.go`):
- Splits context window: 20% system, 40% context, 30% history, 10% response
- `BudgetManager` enforces limits per section
- Immutable operations return new instances

**TUI Message Flow** (`internal/tui/tui.go`):
```
KeyMsg → handleKeyMsg() → handleSubmit() → Provider.Stream()
                                              ↓
StreamChunkMsg ← accumulate text/tool calls ← channel
                                              ↓
                               processToolCalls() → SuggestionView
```

**Search & Indexing** (`internal/search/`):
- SQLite FTS5 with porter stemmer
- Chunks: 800 tokens, 15% overlap
- mtime-based incremental sync

### Data Flow

User input → TUI builds ChatRequest (messages + system prompt with ranked context chunks + tools) → Provider streams response → Tool calls trigger SuggestionHandler → Approval UI → Context file updates

### Project Structure (User Data)

```
~/.config/dreamteller/config.yaml     # Global config (API keys, defaults)
~/.local/share/dreamteller/<project>/
├── .dreamteller/
│   ├── config.yaml                   # Project config
│   └── store.db                      # SQLite (FTS5 + metadata)
├── context/
│   ├── characters/*.md
│   ├── settings/*.md
│   └── plot/*.md
└── chapters/*.md
```

### Critical Files

| File | Purpose |
|------|---------|
| `internal/tui/tui.go` | TUI model, view states, key handling |
| `internal/llm/provider.go` | Provider interface definition |
| `internal/llm/tools.go` | Tool schemas for AI suggestions |
| `internal/token/budget.go` | Token allocation logic |
| `internal/search/fts.go` | FTS5 search implementation |
| `internal/search/indexer.go` | Content chunking pipeline |

## Common Issues

**TUI input not working**: Key messages must pass through to textarea. Check `handleKeyMsg()` returns `nil` cmd for regular keys.

**FTS5 build errors**: Ensure `CGO_ENABLED=1` and `-tags "fts5"` are set.

**Provider errors**: API keys loaded from environment variables (`OPENAI_API_KEY`, `GEMINI_API_KEY`).
