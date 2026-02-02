# Dreamteller

AI와 협업하여 소설을 작성하는 터미널 기반 애플리케이션입니다.

## Features

- **대화형 채팅 인터페이스**: gemini-cli 스타일의 TUI로 AI와 실시간 협업
- **컨텍스트 시스템**: 캐릭터, 배경, 플롯 설정을 마크다운으로 관리하고 자동으로 AI에 주입
- **멀티 프로바이더 지원**: OpenAI, Gemini, 로컬 LLM 어댑터
- **스마트 검색**: FTS5 기반 전문 검색으로 관련 컨텍스트 자동 검색
- **토큰 예산 관리**: 컨텍스트 윈도우를 효율적으로 활용
- **AI 제안 시스템**: 플롯 발전, 캐릭터 행동 제안을 승인/거절

## Installation

```bash
# Clone
git clone https://github.com/yourusername/dreamteller.git
cd dreamteller

# Build (requires CGO for SQLite FTS5)
CGO_ENABLED=1 go build -tags "fts5" -o dreamteller ./cmd/dreamteller

# Install globally (optional)
go install -tags "fts5" ./cmd/dreamteller
```

## Quick Start

```bash
# 새 프로젝트 생성 (Wizard 모드)
dreamteller new my-novel

# 프롬프트 기반 한 방 설정
dreamteller new my-novel --from-prompt prompt.txt

# 프로젝트 열기
dreamteller open my-novel

# 프로젝트 목록
dreamteller list
```

## Configuration

### Global Config (`~/.config/dreamteller/config.yaml`)

```yaml
version: 1
projects_dir: ~/dreamteller-projects

providers:
  openai:
    api_key: ${OPENAI_API_KEY}
    default_model: gpt-4-turbo
  gemini:
    api_key: ${GEMINI_API_KEY}
    default_model: gemini-1.5-pro

defaults:
  provider: openai
```

### Environment Variables

```bash
export OPENAI_API_KEY="sk-..."
export GEMINI_API_KEY="..."
```

## Project Structure

```
my-novel/
├── .dreamteller/
│   ├── config.yaml      # 프로젝트 설정
│   └── store.db         # SQLite (FTS5 + 메타데이터)
├── context/
│   ├── characters/      # 캐릭터 설정 (*.md)
│   ├── settings/        # 배경 설정 (*.md)
│   └── plot/            # 스토리 플롯 (*.md)
├── chapters/            # 작성된 챕터 (*.md)
└── README.md
```

## TUI Commands

| 명령어 | 설명 |
|--------|------|
| `/help` | 도움말 표시 |
| `/clear` | 대화 내역 초기화 |
| `/context` | 현재 컨텍스트 보기 |
| `/search <query>` | 컨텍스트 검색 |
| `/reindex` | 인덱스 재빌드 |
| `/chapter <n>` | 챕터 선택 |
| `Ctrl+C` | 스트리밍 취소 / 종료 |
| `Esc` | 뷰 전환 |

## Architecture

```
cmd/dreamteller/     CLI entry (Cobra)
        ↓
internal/app/        App lifecycle, config
        ↓
internal/tui/        Bubble Tea TUI (Elm architecture)
        ↓
internal/llm/        Provider interface + adapters
        ↓
internal/token/      Token counting + budget
internal/search/     FTS5 full-text search
internal/storage/    SQLite + atomic writes
internal/project/    Project CRUD
        ↓
pkg/types/           Shared domain models
```

## Development

```bash
# Run tests
CGO_ENABLED=1 go test -tags "fts5" ./...

# Run tests with coverage
CGO_ENABLED=1 go test -tags "fts5" -cover ./...

# Build
CGO_ENABLED=1 go build -tags "fts5" -o dreamteller ./cmd/dreamteller
```

## Tech Stack

- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **LLM**: OpenAI API, Google Gemini API
- **Search**: SQLite FTS5
- **CLI**: [Cobra](https://github.com/spf13/cobra)
- **Token Counting**: [tiktoken-go](https://github.com/pkoukk/tiktoken-go)

## License

MIT
