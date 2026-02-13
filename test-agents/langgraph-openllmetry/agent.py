# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "langgraph",
#     "langchain-aws",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "opentelemetry-instrumentation-langchain",
# ]
# ///
"""
Minimal LangGraph agent with OpenLLMetry instrumentation.
Sends traces to OTel Collector at localhost:4317.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

# Configure OTLP export
resource = Resource.create({"service.name": "langgraph-test-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

# Initialize OpenLLMetry instrumentation for LangChain/LangGraph
from opentelemetry.instrumentation.langchain import LangchainInstrumentor
LangchainInstrumentor().instrument()

# Build a minimal LangGraph agent
from langgraph.graph import StateGraph, START, END
from langchain_aws import ChatBedrock
from typing import TypedDict

class State(TypedDict):
    message: str
    response: str

llm = ChatBedrock(
    model_id="us.anthropic.claude-sonnet-4-20250514-v1:0",
    region_name="us-west-2",
)

def call_model(state: State) -> State:
    result = llm.invoke(state["message"])
    return {"response": result.content}

graph = StateGraph(State)
graph.add_node("model", call_model)
graph.add_edge(START, "model")
graph.add_edge("model", END)
app = graph.compile()

# Run the agent
result = app.invoke({"message": "What is 2 + 2? Reply in one word."})
print(f"Response: {result['response']}")

# Flush traces before exit
provider.force_flush()
print("Trace sent to collector at localhost:4317")
