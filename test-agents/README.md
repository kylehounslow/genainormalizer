# Test Agents for Data Prepper Trace Processing

Real agent implementations with real instrumentation libraries to test Data Prepper trace processing.

## Setup

Each agent folder has its own `requirements.txt`. Use `uv` to run:

```bash
cd <agent-folder>
uv run agent.py
```

## Agents

| Folder | Agent Framework | Instrumentation | Export Path | Status |
|--------|-----------------|-----------------|-------------|--------|
| `strands-openllmetry/` | Strands | Native + Bedrock | → Data Prepper (gRPC 21890) | ✅ Ready |
| `langgraph-openllmetry/` | LangGraph | OpenLLMetry | → Data Prepper (gRPC 21890) | ✅ Ready |
| `langchain-openllmetry/` | LangChain | OpenLLMetry | → Data Prepper (gRPC 21890) | ✅ Ready |
| `crewai-openllmetry/` | CrewAI | OpenLLMetry | → Data Prepper (gRPC 21890) | ✅ Ready |
| `strands-openinference/` | Strands | OpenInference | → OTel Collector (HTTP 4318) → Data Prepper | ⚠️ OS mapping conflict |
| `langgraph-openinference/` | LangGraph | OpenInference | → Data Prepper (gRPC 21890) | ⚠️ OS mapping conflict |
| `langchain-openinference/` | LangChain | OpenInference | → Data Prepper (gRPC 21890) | ⚠️ OS mapping conflict |
| `crewai-openinference/` | CrewAI | OpenInference | → Data Prepper (gRPC 21890) | ⚠️ OS mapping conflict |

### OpenSearch Mapping Conflict (OpenInference)

OpenInference emits `llm.input_messages` as both a JSON string AND flattened dotted sub-keys
(e.g. `llm.input_messages.0.message.content`). OpenSearch rejects these documents with:
```
object mapping for [attributes.llm.input_messages] tried to parse field
[llm.input_messages] as object, but found a concrete value
```
See `.steering/genai-trace-attribute-analysis.md` for full details.

## Target

All agents export traces to Data Prepper OTLP endpoint: `localhost:21890`

## Debugging Workflow

1. Start Data Prepper in IntelliJ (Debug mode)
2. Set breakpoint in `OTelTraceRawProcessor.doExecute()`
3. Run agent: `uv run agent.py`
4. Inspect trace data at breakpoint
