# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "strands-agents",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-http",
#     "openinference-instrumentation-strands-agents",
# ]
# ///
"""
Minimal Strands agent with OpenInference instrumentation.
"""

from strands.telemetry import StrandsTelemetry
from openinference.instrumentation.strands_agents import StrandsAgentsToOpenInferenceProcessor

telemetry = StrandsTelemetry()
telemetry.setup_otlp_exporter(endpoint="http://localhost:4318/v1/traces")
telemetry.tracer_provider.add_span_processor(StrandsAgentsToOpenInferenceProcessor())

from strands import Agent
from strands.models import BedrockModel

model = BedrockModel(
    model_id="us.anthropic.claude-sonnet-4-20250514-v1:0",
    region_name="us-west-2",
)

agent = Agent(model=model)
response = agent("What is 2 + 2? Reply in one word.")
print(f"Response: {response}")

# Ensure traces are exported
import time
telemetry.tracer_provider.force_flush(timeout_millis=10000)
time.sleep(2)
telemetry.tracer_provider.shutdown()
