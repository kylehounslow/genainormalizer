# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "langchain",
#     "langchain-aws",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "openinference-instrumentation-langchain",
# ]
# ///
"""
Minimal LangChain agent with OpenInference instrumentation.
Sends traces to OTel Collector at localhost:4317.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

resource = Resource.create({"service.name": "langchain-openinference-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(SimpleSpanProcessor(exporter))
trace.set_tracer_provider(provider)

from openinference.instrumentation.langchain import LangChainInstrumentor
LangChainInstrumentor().instrument()

from langchain_aws import ChatBedrock

llm = ChatBedrock(
    model_id="us.anthropic.claude-sonnet-4-20250514-v1:0",
    region_name="us-west-2",
)

response = llm.invoke("What is 2 + 2? Reply in one word.")
print(f"Response: {response.content}")

provider.force_flush()
print("Trace sent to collector at localhost:4317")
