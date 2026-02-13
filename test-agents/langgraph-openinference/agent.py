# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "langgraph",
#     "langchain-aws",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "openinference-instrumentation-langchain",
# ]
# ///
"""
Minimal LangGraph agent with OpenInference instrumentation.
Sends traces to Data Prepper at localhost:4317.
"""

# Setup tracing FIRST before any LangChain imports
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

resource = Resource.create({"service.name": "langgraph-openinference-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(SimpleSpanProcessor(exporter))
trace.set_tracer_provider(provider)

# Initialize OpenInference instrumentation BEFORE importing LangChain
from openinference.instrumentation.langchain import LangChainInstrumentor
LangChainInstrumentor().instrument()

# NOW import LangChain/LangGraph
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

result = app.invoke({"message": "What is 2 + 2? Reply in one word."})
print(f"Response: {result['response']}")

provider.force_flush()
print("Trace sent to Data Prepper at localhost:4317")
