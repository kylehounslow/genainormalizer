package genainormalizer

// Mapping defines a source attribute key and its normalized target.
type Mapping struct {
	From      string
	To        string
	WrapSlice bool // when true, wrap scalar string value into a single-element string slice
}

// MappingTarget holds the resolved target for a source attribute.
type MappingTarget struct {
	Key       string
	WrapSlice bool
}

// Profile returns the attribute mappings for a given profile name.
func Profile(name string) []Mapping {
	switch name {
	case "openinference":
		return openInferenceMappings
	case "openllmetry":
		return openLLMetryMappings
	case "langchain":
		return langchainMappings
	case "crewai":
		return crewaiMappings
	case "pydanticai":
		return pydanticaiMappings
	case "strands":
		return strandsMappings
	case "llamaindex":
		return llamaIndexMappings
	case "langsmith":
		return langsmithMappings
	case "autogen":
		return autogenMappings
	default:
		return nil
	}
}

// BuildLookupTable merges enabled profiles into a single map for O(1) lookup.
// If multiple profiles map the same source attribute to different targets, the last profile wins.
func BuildLookupTable(profiles []string) map[string]MappingTarget {
	table := make(map[string]MappingTarget)
	for _, name := range profiles {
		for _, m := range Profile(name) {
			table[m.From] = MappingTarget{Key: m.To, WrapSlice: m.WrapSlice}
		}
	}
	return table
}

// --- OpenInference (Arize) ---
// Ref: https://github.com/Arize-ai/openinference/blob/main/spec/semantic_conventions.md

var openInferenceMappings = []Mapping{
	// Token usage
	{From: "llm.token_count.prompt", To: "gen_ai.usage.input_tokens"},
	{From: "llm.token_count.completion", To: "gen_ai.usage.output_tokens"},

	// Model & provider
	{From: "llm.model_name", To: "gen_ai.request.model"},
	{From: "llm.provider", To: "gen_ai.provider.name"},

	// Input/output content
	{From: "llm.input_messages", To: "gen_ai.input.messages"},
	{From: "llm.output_messages", To: "gen_ai.output.messages"},

	// Embeddings
	{From: "embedding.model_name", To: "gen_ai.request.model"},

	// Tool
	{From: "tool.name", To: "gen_ai.tool.name"},
	{From: "tool.description", To: "gen_ai.tool.description"},
	{From: "tool_call.function.arguments", To: "gen_ai.tool.call.arguments"},
	{From: "tool_call.id", To: "gen_ai.tool.call.id"},

	// Reranker model (mutually exclusive with llm/embedding model)
	{From: "reranker.model_name", To: "gen_ai.request.model"},

	// Agent & session
	{From: "agent.name", To: "gen_ai.agent.name"},
	{From: "session.id", To: "gen_ai.conversation.id"},

	// Span kind → operation name (value mapping handled separately)
	{From: "openinference.span.kind", To: "gen_ai.operation.name"},
}

// --- OpenLLMetry (Traceloop) ---
// Ref: https://www.traceloop.com/docs/openllmetry/contributing/semantic-conventions

var openLLMetryMappings = []Mapping{
	// Token usage
	{From: "llm.usage.prompt_tokens", To: "gen_ai.usage.input_tokens"},
	{From: "llm.usage.completion_tokens", To: "gen_ai.usage.output_tokens"},

	// Model & provider
	{From: "llm.request.model", To: "gen_ai.request.model"},
	{From: "llm.response.model", To: "gen_ai.response.model"},

	// Request params
	{From: "llm.request.max_tokens", To: "gen_ai.request.max_tokens"},
	{From: "llm.request.temperature", To: "gen_ai.request.temperature"},
	{From: "llm.request.top_p", To: "gen_ai.request.top_p"},
	{From: "llm.top_k", To: "gen_ai.request.top_k"},
	{From: "llm.frequency_penalty", To: "gen_ai.request.frequency_penalty"},
	{From: "llm.presence_penalty", To: "gen_ai.request.presence_penalty"},
	{From: "llm.chat.stop_sequences", To: "gen_ai.request.stop_sequences"},
	{From: "llm.request.functions", To: "gen_ai.tool.definitions"},

	// Response — string to string[] conversion
	{From: "llm.response.finish_reason", To: "gen_ai.response.finish_reasons", WrapSlice: true},
	{From: "llm.response.stop_reason", To: "gen_ai.response.finish_reasons", WrapSlice: true},

	// Operation — llm.request.type on LLM spans, traceloop.span.kind on workflow spans
	{From: "llm.request.type", To: "gen_ai.operation.name"},
	{From: "traceloop.span.kind", To: "gen_ai.operation.name"},

	// Traceloop workflow/entity (agentic)
	{From: "traceloop.entity.name", To: "gen_ai.agent.name"},
	{From: "traceloop.entity.input", To: "gen_ai.input.messages"},
	{From: "traceloop.entity.output", To: "gen_ai.output.messages"},
}

// --- LangChain ---

var langchainMappings = []Mapping{}

// --- CrewAI ---

var crewaiMappings = []Mapping{}

// --- PydanticAI ---

var pydanticaiMappings = []Mapping{}

// --- Strands (AWS) ---
// Strands emits gen_ai.* attributes natively. No framework-specific
// attributes exist in real telemetry. This profile is intentionally
// empty — Strands spans are normalized by the openinference/openllmetry
// profiles (when using those instrumentors) or are already gen_ai.*
// compliant (when using Strands' native telemetry).

var strandsMappings = []Mapping{}

// --- LlamaIndex ---
// Ref: https://docs.llamaindex.ai/en/stable/module_guides/observability/instrumentation/
// LlamaIndex uses OpenInference semantic conventions (both from Arize).
// This profile extends OpenInference with LlamaIndex-specific mappings and
// includes comprehensive token count details for OpenAI's newer breakdowns.

var llamaIndexMappings = []Mapping{
	// ===== Core Token Usage =====
	{From: "llm.token_count.prompt", To: "gen_ai.usage.input_tokens"},
	{From: "llm.token_count.completion", To: "gen_ai.usage.output_tokens"},
	{From: "llm.token_count.total", To: "gen_ai.usage.total_tokens"},

	// ===== Token Count Details (OpenAI breakdowns) =====
	// Prompt details
	{From: "llm.token_count.prompt_details.audio", To: "gen_ai.usage.prompt_details.audio_tokens"},
	{From: "llm.token_count.prompt_details.cache_read", To: "gen_ai.usage.prompt_details.cached_tokens"},

	// Completion details
	{From: "llm.token_count.completion_details.audio", To: "gen_ai.usage.completion_details.audio_tokens"},
	{From: "llm.token_count.completion_details.reasoning", To: "gen_ai.usage.completion_details.reasoning_tokens"},

	// ===== Model & Provider =====
	{From: "llm.model_name", To: "gen_ai.request.model"},
	{From: "llm.response.model", To: "gen_ai.response.model"},
	{From: "llm.provider", To: "gen_ai.system"},
	{From: "llm.provider", To: "gen_ai.provider.name"},
	{From: "llm.system", To: "gen_ai.system"},

	// ===== Response Metadata =====
	{From: "llm.response.id", To: "gen_ai.response.id"},

	// ===== Input/Output Messages =====
	// Top-level message arrays (if present as JSON)
	//{From: "llm.input_messages", To: "gen_ai.input.messages"},
	//{From: "llm.output_messages", To: "gen_ai.output.messages"},

	// Note: Nested message attributes (llm.input_messages.N.message.*)
	// are handled by normalizeNestedMessages() in processor.go

	// ===== Request Parameters =====
	// llm.invocation_parameters: JSON string emitted by OpenInference on LLM spans.
	// Contains the full set of parameters passed to the LLM call, e.g.:
	//   {"temperature":0,"max_tokens":1024,"stream_options":{"include_usage":true}}
	// Mapped to gen_ai.request.parameters as-is (unparsed JSON string).
	{From: "llm.invocation_parameters", To: "gen_ai.request.parameters"},

	// ===== Tools & Function Calling =====
	// Flat tool attributes — emitted on TOOL spans (openinference.span.kind=TOOL).
	// tool.name:        the function/tool name, e.g. "multiply"
	// tool.description: human-readable description of what the tool does
	// tool.parameters:  JSON schema of the tool's input parameters
	{From: "tool.name", To: "gen_ai.tool.name"},
	{From: "tool.description", To: "gen_ai.tool.description"},
	{From: "tool.parameters", To: "gen_ai.tool.parameters"},

	// Nested tool definitions — emitted on LLM spans as llm.tools.N.tool.*
	// These are collected into a JSON array by normalizeNestedToolDefinitions() in processor.go
	// and written to gen_ai.tool.definitions.
	// Example source attrs:
	//   llm.tools.0.tool.json_schema → {"name":"multiply","description":"...","parameters":{...}}
	//   llm.tools.1.tool.json_schema → {"name":"divide","description":"...","parameters":{...}}

	// Tool call execution — emitted on LLM spans when the model invokes a tool.
	// tool_call.function.name:      name of the tool being called, e.g. "multiply"
	// tool_call.function.arguments: JSON string of arguments, e.g. {"a":2,"b":3}
	// tool_call.id:                 OpenAI tool call ID, e.g. "call_abc123"
	{From: "tool_call.function.name", To: "gen_ai.tool.call.name"},
	{From: "tool_call.function.arguments", To: "gen_ai.tool.call.arguments"},
	{From: "tool_call.id", To: "gen_ai.tool.call.id"},

	// Tool call result — emitted on TOOL spans after tool execution completes.
	// Contains the raw return value from the tool function, e.g. "6.0"
	{From: "tool_call.result", To: "gen_ai.tool.call.result"},

	// Tool type
	{From: "tool.type", To: "gen_ai.tool.type"},

	// ===== Embeddings =====
	{From: "embedding.model_name", To: "gen_ai.request.model"},

	// ===== Reranker =====
	{From: "reranker.model_name", To: "gen_ai.request.model"},

	// ===== Agent & Workflow =====
	{From: "agent.name", To: "gen_ai.agent.name"},
	{From: "agent.type", To: "gen_ai.agent.type"},

	// ===== Session & Conversation =====
	{From: "session.id", To: "gen_ai.conversation.id"},

	// ===== Operation Type =====
	// Span kind → operation name (value mapping handled in valuemappings.go)
	{From: "openinference.span.kind", To: "gen_ai.operation.name"},

	// ===== LlamaIndex-specific attributes =====
	// Query/request content
	{From: "query", To: "gen_ai.prompt"},
	{From: "user.prompt", To: "gen_ai.prompt"},
	{From: "user_input", To: "gen_ai.prompt"},

	// Response content
	{From: "response", To: "gen_ai.completion"},
	{From: "response.content", To: "gen_ai.completion"},
	{From: "answer", To: "gen_ai.completion"},

	// Request/response generic fields
	//{From: "request", To: "gen_ai.input.messages"},

	// Category for operation classification
	{From: "category", To: "gen_ai.operation.category"},
}

// --- AutoGen (Microsoft) ---
// Source: AutoGen's built-in OTEL instrumentation (autogen-agentchat / autogen-core)
// InstrumentationScope: "autogen SingleThreadedAgentRuntime"
// AutoGen uses OpenTelemetry messaging semantic conventions for agent communication.
// Ref: https://microsoft.github.io/autogen/stable/user-guide/core-user-guide/framework/telemetry.html
//
// Span types emitted:
//   "autogen create <agent>" - agent creation (messaging.operation: create)
//   "autogen send <agent>"   - message dispatch (messaging.operation: publish)
//   "autogen ack"            - acknowledgment   (messaging.operation: receive)
//
// Special handling: messaging.destination values like "math_tutor.(default)-A" are
// cleaned to "math_tutor" by normalizeAutoGenAttributes() in processor.go.

var autogenMappings = []Mapping{
	// ===== AutoGen runtime spans (InstrumentationScope: "autogen SingleThreadedAgentRuntime") =====

	// Agent identity: the destination is the agent receiving the message.
	// Value cleanup (strip instance suffix) is done in processor.go.
	{From: "messaging.destination", To: "gen_ai.agent.name"},

	// Operation type: create/publish/receive → create_agent/send_message/receive_message
	// Value mapping defined in valuemappings.go.
	{From: "messaging.operation", To: "gen_ai.operation.name"},

	// Message type (e.g., UserMessage, AssistantMessage, TextMessage)
	{From: "messaging.message.type", To: "gen_ai.message.type"},

	// ===== OpenAI instrumentation spans (InstrumentationScope: "opentelemetry.instrumentation.openai.v1") =====
	// AutoGen uses opentelemetry-instrumentation-openai alongside its own runtime tracing.
	// These spans carry LLM call details and emit a mix of gen_ai.* and legacy llm.* attributes.

	// Token usage
	{From: "llm.usage.prompt_tokens", To: "gen_ai.usage.input_tokens"},
	{From: "llm.usage.completion_tokens", To: "gen_ai.usage.output_tokens"},
	{From: "llm.usage.total_tokens", To: "gen_ai.usage.total_tokens"},

	// Model
	{From: "llm.request.model", To: "gen_ai.request.model"},
	{From: "llm.response.model", To: "gen_ai.response.model"},

	// Operation type: "chat" → "chat" (value passthrough via valuemappings.go)
	{From: "llm.request.type", To: "gen_ai.operation.name"},

	// Request parameters
	{From: "llm.request.temperature", To: "gen_ai.request.temperature"},
	{From: "llm.request.max_tokens", To: "gen_ai.request.max_tokens"},
	{From: "llm.request.top_p", To: "gen_ai.request.top_p"},

	// Note: llm.request.functions.N.* (indexed tool definitions) are handled by
	// extractToolDefinitionsFromFunctions() in processor.go, not via simple key renames.
}

// --- LangSmith ---
// Source: LangSmith observability platform (LangChain's official OTEL instrumentation)
// InstrumentationScope: langsmith, src.app
// Ref: https://docs.smith.langchain.com/
// Note: LangSmith already emits most gen_ai.* attributes natively.
// These mappings handle LangSmith-specific metadata and custom application spans.

var langsmithMappings = []Mapping{
	// ===== LangSmith Metadata → Standard Attributes =====
	// Session/Project tracking
	{From: "langsmith.trace.session_name", To: "gen_ai.conversation.id"},
	{From: "langsmith.metadata.LANGSMITH_PROJECT", To: "gen_ai.conversation.id"},

	// Model metadata (fallback if gen_ai.request.model not present)
	{From: "langsmith.metadata.ls_model_name", To: "gen_ai.request.model"},
	// Note: langsmith.metadata.ls_provider is NOT mapped to gen_ai.system here.
	// It contains "langchain" for chain/agent spans, which is the framework name, not the LLM provider.
	// gen_ai.system propagation is handled in processor.go (propagated from LLM spans).

	// Request parameters
	{From: "langsmith.metadata.ls_temperature", To: "gen_ai.request.temperature"},
	{From: "langsmith.metadata.ls_model_type", To: "gen_ai.operation.name"},

	// ===== Generic Application Spans (src.app, etc.) =====
	// For custom instrumentation that uses simple attribute names
	{From: "user.prompt", To: "gen_ai.prompt"},
	{From: "user_input", To: "gen_ai.prompt"},
	{From: "query", To: "gen_ai.prompt"},
	{From: "request", To: "gen_ai.input.messages"},

	{From: "response.content", To: "gen_ai.completion"},
	{From: "response", To: "gen_ai.completion"},
	{From: "answer", To: "gen_ai.completion"},
	{From: "completion", To: "gen_ai.completion"},

	// Model & Provider (generic names)
	{From: "model", To: "gen_ai.request.model"},
	{From: "model_name", To: "gen_ai.request.model"},
	{From: "provider", To: "gen_ai.provider.name"},

	// Agent/Chain/Conversation (generic names)
	{From: "agent.name", To: "gen_ai.agent.name"},
	{From: "agent.type", To: "gen_ai.agent.type"},
	{From: "agent_role", To: "gen_ai.agent.name"},
	{From: "chain.name", To: "gen_ai.agent.name"},
	{From: "session.id", To: "gen_ai.conversation.id"},
	{From: "conversation.id", To: "gen_ai.conversation.id"},
	{From: "thread.id", To: "gen_ai.conversation.id"},

	// Operation Type
	{From: "category", To: "gen_ai.operation.name"},
	{From: "operation", To: "gen_ai.operation.name"},

	// Tools
	{From: "tool.name", To: "gen_ai.tool.name"},
	{From: "tool.description", To: "gen_ai.tool.description"},
	{From: "tool.parameters", To: "gen_ai.tool.parameters"},

	// Note: langsmith.span.kind and langsmith.trace.name are preserved as-is
	// They provide additional context beyond the standard gen_ai.* attributes
}
