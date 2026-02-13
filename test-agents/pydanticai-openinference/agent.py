# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "pydantic-ai[bedrock]",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-http",
#     "openinference-instrumentation-pydantic-ai",
# ]
# ///
"""
Minimal PydanticAI agent with OpenInference instrumentation.
Sends traces to OTel Collector at localhost:4318.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from openinference.instrumentation.pydantic_ai import OpenInferenceSpanProcessor

resource = Resource.create({"service.name": "pydanticai-openinference-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="http://localhost:4318/v1/traces")
provider.add_span_processor(OpenInferenceSpanProcessor())
provider.add_span_processor(SimpleSpanProcessor(exporter))
trace.set_tracer_provider(provider)

from pydantic_ai import Agent

# Enable PydanticAI's built-in OTel instrumentation
Agent.instrument_all()

agent = Agent(
    "bedrock:us.anthropic.claude-sonnet-4-20250514-v1:0",
    system_prompt="You are a helpful assistant. Reply concisely.",
)

result = agent.run_sync("What is 2 + 2? Reply in one word.")
print(f"Response: {result.output}")

provider.force_flush()
provider.shutdown()
print("Trace sent to collector at localhost:4318")
