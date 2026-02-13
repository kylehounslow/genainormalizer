# Test Agents for genainormalizer Validation

Real agent implementations with real instrumentation libraries to validate the genainormalizer processor end-to-end.

## Setup

Each agent folder is a standalone `uv` script. Run with:

```bash
cd <agent-folder>
uv run agent.py
```

Requires AWS credentials for Bedrock (Claude Sonnet 4).

## Agents

| Folder | Agent Framework | Instrumentation | Status |
|--------|-----------------|-----------------|--------|
| `strands-openinference/` | Strands | OpenInference | ✅ |
| `strands-openllmetry/` | Strands | OpenLLMetry | ✅ |
| `langchain-openinference/` | LangChain | OpenInference | ✅ |
| `langchain-openllmetry/` | LangChain | OpenLLMetry | ✅ |
| `langgraph-openinference/` | LangGraph | OpenInference | ✅ |
| `langgraph-openllmetry/` | LangGraph | OpenLLMetry | ✅ |
| `crewai-openinference/` | CrewAI | OpenInference | ✅ |
| `crewai-openllmetry/` | CrewAI | OpenLLMetry | ✅ |
| `pydanticai-openinference/` | PydanticAI | OpenInference | ✅ |

## Target

All agents export traces to OTel Collector OTLP endpoint: `localhost:4317` (gRPC) or `localhost:4318` (HTTP).

## Running All Tests

```bash
# Build the collector first
builder --config builder-config.yaml

# Run all scenarios
./run-all.sh
```
