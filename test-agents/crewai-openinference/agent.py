# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "crewai[bedrock]",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "openinference-instrumentation-crewai",
# ]
# ///
"""
Minimal CrewAI agent with OpenInference instrumentation.
Sends traces to OTel Collector at localhost:4317.
"""
import os
os.environ["CREWAI_TRACING_ENABLED"] = "false"  # Disable CrewAI's interactive prompt

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.resources import Resource
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

resource = Resource.create({"service.name": "crewai-openinference-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(SimpleSpanProcessor(exporter))
trace.set_tracer_provider(provider)

from openinference.instrumentation.crewai import CrewAIInstrumentor
CrewAIInstrumentor().instrument()

from crewai import Agent, Task, Crew, LLM

llm = LLM(
    model="bedrock/us.anthropic.claude-sonnet-4-20250514-v1:0",
    region_name="us-west-2",
)

agent = Agent(
    role="Math Expert",
    goal="Answer math questions",
    backstory="You are a math expert.",
    llm=llm,
)

task = Task(
    description="What is 2 + 2? Reply in one word.",
    expected_output="A single word answer",
    agent=agent,
)

crew = Crew(agents=[agent], tasks=[task], verbose=False)
result = crew.kickoff()
print(f"Response: {result.raw}")

provider.force_flush()
print("Trace sent to collector at localhost:4317")
