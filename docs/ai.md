# AI Assistant

Mimir can delegate analysis to an LLM. The assistant is optional — every `ai`
command errors clearly if AI is not configured.

```
AI not configured. Set ai.provider and ai.model in config.yaml or via env vars
```

## Commands

| Command | Description |
|---|---|
| `ai ask <question>` | Ask a question with case context |
| `ai analyze` | Summarize the current case findings |
| `ai suggest` | Recommend next investigation steps |
| `ai explain <text>` | Explain tool output or a concept |
| `ai summarize` | Generate a case narrative (stdout) |
| `ai config` | Show the active AI configuration (read-only) |
| `ai models` | List models the provider can serve |
| `ai use <model> [--save]` | Switch the active model; `--save` persists to `config.yaml` |

`ai config` redacts the API key; `ai models` marks the active model with `*`;
`ai use` validates against the provider's model list when discovery is
supported (`ai models` otherwise still works via the current model).

Every interaction is recorded on the case timeline as `ai_query` and
`ai_response` events.

## Providers

| Provider | Default base URL | Notes |
|---|---|---|
| `openrouter` | `https://openrouter.ai/api/v1` | Default; wide model choice |
| `anthropic` | provider default | Claude models |
| `ollama` | `http://localhost:11434/v1` | **Local** — gets unmasked case context |
| custom / any | set `base_url` | Any OpenAI-compatible endpoint |

**Context handling**: local providers (`ollama`, self-hosted) receive full case
context, including evidence filenames. Remote providers receive
PII-safe context with evidence filenames masked.

## Configuration

Config keys (see [Overview & configuration](overview.md)):

```
ai.provider       # openrouter | anthropic | ollama | custom
ai.base_url
ai.api_key
ai.model
ai.max_tokens     # 4096
ai.temperature    # 0.1
ai.context_window # 50000 (token budget for context)
ai.timeout        # 60 (seconds)
ai.auto_analyze_evidence  # true → auto-AI note when evidence is added
```

Equivalent env vars: `MIMIR_AI_PROVIDER`, `MIMIR_AI_BASE_URL`,
`MIMIR_AI_API_KEY`, `MIMIR_AI_MODEL`, `MIMIR_AI_MAX_TOKENS`,
`MIMIR_AI_TEMPERATURE`, `MIMIR_AI_CONTEXT_WINDOW`, `MIMIR_AI_TIMEOUT`.

## Workflow

```
ai use anthropic/claude-sonnet-4-20250514 --save
evidence add sample.bin --tag malware
ai analyze
ai suggest
ai explain <pasted tool output>
ai summarize
report                     # embeds the AI narrative unless --no-ai
```
