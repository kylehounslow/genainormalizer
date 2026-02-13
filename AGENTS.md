# AGENTS.md

## Project

`genainormalizer` — An OpenTelemetry Collector processor that normalizes GenAI telemetry attributes from multiple instrumentation libraries (OpenInference, OpenLLMetry) and frameworks (LangChain, CrewAI, PydanticAI, Strands) into the official [OTel GenAI Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/).

## Owner

- **Author:** Kyle Hounslow (@kylehounslow)
- **GitHub:** https://github.com/kylehounslow/genainormalizer

## Architecture

```
Agent (Python/TS)
  → OTel SDK (OpenInference / OpenLLMetry / native)
    → OTel Collector + genainormalizer processor
      → Any OTLP-compatible backend
```

The processor sits in the Collector pipeline and normalizes attributes before they reach any backend.

## Language & Build

- **Language:** Go
- **Build:** OpenTelemetry Collector Builder (OCB) to compile a custom collector binary
- **Test:** `go test ./...`

## Key Design Decisions

1. **Standalone Go module** — Not a fork of otel-collector-contrib. Uses OCB to build a custom collector. Will be donated to contrib later.
2. **Profile-based mapping** — Each instrumentation library/framework is a "profile" that can be enabled/disabled independently.
3. **Single-pass O(1) lookup** — Pre-allocated hash map, no sequential rule evaluation.
4. **Value mapping** — Span kind values (e.g. `LLM` → `chat`, `AGENT` → `invoke_agent`) are transformed, not just keys.
5. **remove_originals option** — Optionally delete source attributes after mapping (default: true).
6. **No-op on non-GenAI spans** — Early exit when no recognized attributes found.

## Mapping Categories

### 1. Schema Collision Resolution
OpenInference and OpenLLMetry use different attribute names for the same concepts. Map both to OTel GenAI SemConv.

Example: `llm.usage.prompt_tokens` (OpenLLMetry) and `llm.token_count.prompt` (OpenInference) → `gen_ai.usage.input_tokens`

### 2. Metadata Promotion (LangChain/LangGraph)
LangChain nests identifiers in JSON metadata maps. Parse once, promote to first-class attributes.

Example: `lc.metadata.thread_id` → `gen_ai.conversation.id`

### 3. Agentic Abstraction Unification
Framework-specific agent/task keys mapped to standard GenAI conventions.

Example: `crewai.agent.role`, `pydantic_ai.agent.name`, `strands.agent.name` → `gen_ai.agent.name`

### 4. Value Mapping
Span kind enums mapped to OTel `gen_ai.operation.name` well-known values.

Example: `openinference.span.kind: "LLM"` → `gen_ai.operation.name: "chat"`

## Canonical Attribute References

- [OTel GenAI Spans](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-spans/)
- [OTel GenAI Agent Spans](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/)
- [OTel GenAI SemConv Registry](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/)
- [OpenInference spec](https://github.com/Arize-ai/openinference/blob/main/spec/semantic_conventions.md)
- [OpenLLMetry instrumentations](https://github.com/traceloop/openllmetry/tree/main/packages)

## Conventions

- Go module path: `github.com/kylehounslow/genainormalizer`
- Branch strategy: `main` for releases, feature branches for development
