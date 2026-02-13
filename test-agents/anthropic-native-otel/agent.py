# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "anthropic[bedrock]",
#     "boto3",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-http",
#     "opentelemetry-instrumentation-anthropic",
# ]
# ///
"""
Anthropic SDK with official OTel GenAI instrumentation.
Emits gen_ai.* attributes natively — processor should be a no-op.
Sends traces to OTel Collector at localhost:4318.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

resource = Resource.create({"service.name": "anthropic-native-otel-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="http://localhost:4318/v1/traces")
provider.add_span_processor(SimpleSpanProcessor(exporter))
trace.set_tracer_provider(provider)

from opentelemetry.instrumentation.anthropic import AnthropicInstrumentor
AnthropicInstrumentor().instrument()

import anthropic

client = anthropic.AnthropicBedrock(aws_region="us-west-2")

message = client.messages.create(
    model="us.anthropic.claude-sonnet-4-20250514-v1:0",
    max_tokens=10,
    messages=[{"role": "user", "content": "What is 2 + 2? Reply in one word."}],
)
print(f"Response: {message.content[0].text}")

provider.force_flush()
provider.shutdown()
print("Trace sent to collector at localhost:4318")
