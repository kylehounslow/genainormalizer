# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "strands-agents",
#     "strands-agents-tools",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "opentelemetry-instrumentation-bedrock",
# ]
# ///
"""
Minimal Strands agent with OpenLLMetry instrumentation.
Sends traces to OTel Collector at localhost:4317.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

# Configure OTLP export
resource = Resource.create({"service.name": "strands-test-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

# Initialize OpenLLMetry instrumentation for Bedrock
from opentelemetry.instrumentation.bedrock import BedrockInstrumentor
BedrockInstrumentor().instrument()

# Now create and run the Strands agent
from strands import Agent
from strands.models import BedrockModel

model = BedrockModel(
    model_id="us.anthropic.claude-sonnet-4-20250514-v1:0",
    region_name="us-west-2",
)

agent = Agent(model=model)

# Simple task to generate a trace
response = agent("What is 2 + 2? Reply in one word.")
print(f"Response: {response}")

# Flush traces before exit
provider.force_flush()
print("Trace sent to collector at localhost:4317")
