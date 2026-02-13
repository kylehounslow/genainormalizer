# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "crewai",
#     "langchain-aws",
#     "opentelemetry-api",
#     "opentelemetry-sdk",
#     "opentelemetry-exporter-otlp-proto-grpc",
#     "opentelemetry-instrumentation-crewai",
#     "opentelemetry-instrumentation-langchain",
# ]
# ///
"""
Minimal CrewAI agent with OpenLLMetry instrumentation.
Sends traces to OTel Collector at localhost:4317.
"""

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource

# Configure OTLP export
resource = Resource.create({"service.name": "crewai-test-agent"})
provider = TracerProvider(resource=resource)
exporter = OTLPSpanExporter(endpoint="localhost:4317", insecure=True)
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

# Initialize OpenLLMetry instrumentation
from opentelemetry.instrumentation.crewai import CrewAIInstrumentor
from opentelemetry.instrumentation.langchain import LangchainInstrumentor
CrewAIInstrumentor().instrument()
LangchainInstrumentor().instrument()

# Build minimal CrewAI agent
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

# Flush traces before exit
provider.force_flush()
print("Trace sent to collector at localhost:4317")
