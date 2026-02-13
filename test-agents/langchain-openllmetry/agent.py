# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "langchain",
#     "langchain-aws",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "opentelemetry-instrumentation-langchain",
# ]
# ///
"""
Minimal LangChain agent with OpenLLMetry instrumentation.
Sends traces to Data Prepper at localhost:4317.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

# Configure OTLP export to Data Prepper
resource = Resource.create({"service.name": "langchain-test-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

# Initialize OpenLLMetry instrumentation for LangChain
from opentelemetry.instrumentation.langchain import LangchainInstrumentor
LangchainInstrumentor().instrument()

# Simple LangChain invocation
from langchain_aws import ChatBedrock

llm = ChatBedrock(
    model_id="us.anthropic.claude-sonnet-4-20250514-v1:0",
    region_name="us-west-2",
)

response = llm.invoke("What is 2 + 2? Reply in one word.")
print(f"Response: {response.content}")

# Flush traces before exit
provider.force_flush()
print("Trace sent to Data Prepper at localhost:4317")
