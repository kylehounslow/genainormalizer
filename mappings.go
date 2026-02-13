package genainormalizer

// Mapping defines a source attribute key and its normalized target.
type Mapping struct {
	From string
	To   string
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
	default:
		return nil
	}
}

// BuildLookupTable merges enabled profiles into a single map for O(1) lookup.
func BuildLookupTable(profiles []string) map[string]string {
	table := make(map[string]string)
	for _, name := range profiles {
		for _, m := range Profile(name) {
			table[m.From] = m.To
		}
	}
	return table
}

// --- OpenInference (Arize) ---
// Ref: https://github.com/Arize-ai/openinference/blob/main/spec/semantic_conventions.md

var openInferenceMappings = []Mapping{
	// Token usage
	{"llm.token_count.prompt", "gen_ai.usage.input_tokens"},
	{"llm.token_count.completion", "gen_ai.usage.output_tokens"},

	// Model & provider
	{"llm.model_name", "gen_ai.request.model"},
	{"llm.provider", "gen_ai.provider.name"},
	{"llm.system", "gen_ai.system"},

	// Input/output content
	{"llm.input_messages", "gen_ai.input.messages"},
	{"llm.output_messages", "gen_ai.output.messages"},

	// Embeddings
	{"embedding.model_name", "gen_ai.request.model"},

	// Tool
	{"tool.name", "gen_ai.tool.name"},
	{"tool.description", "gen_ai.tool.description"},
	{"tool_call.function.arguments", "gen_ai.tool.call.arguments"},
	{"tool_call.id", "gen_ai.tool.call.id"},

	// Reranker model (mutually exclusive with llm/embedding model)
	{"reranker.model_name", "gen_ai.request.model"},

	// Agent & session
	{"agent.name", "gen_ai.agent.name"},
	{"session.id", "gen_ai.conversation.id"},

	// Span kind → operation name (value mapping handled separately)
	{"openinference.span.kind", "gen_ai.operation.name"},
}

// --- OpenLLMetry (Traceloop) ---
// Ref: https://github.com/traceloop/openllmetry/blob/main/packages/opentelemetry-semantic-conventions-ai/opentelemetry/semconv_ai/__init__.py

var openLLMetryMappings = []Mapping{
	// Token usage
	{"llm.usage.prompt_tokens", "gen_ai.usage.input_tokens"},
	{"llm.usage.completion_tokens", "gen_ai.usage.output_tokens"},

	// Model & provider
	{"llm.request.model", "gen_ai.request.model"},
	{"llm.response.model", "gen_ai.response.model"},
	{"llm.system", "gen_ai.system"},

	// Request params
	{"llm.request.max_tokens", "gen_ai.request.max_tokens"},
	{"llm.request.temperature", "gen_ai.request.temperature"},
	{"llm.request.top_p", "gen_ai.request.top_p"},
	{"llm.top_k", "gen_ai.request.top_k"},
	{"llm.frequency_penalty", "gen_ai.request.frequency_penalty"},
	{"llm.presence_penalty", "gen_ai.request.presence_penalty"},
	{"llm.chat.stop_sequences", "gen_ai.request.stop_sequences"},
	{"llm.request.functions", "gen_ai.tool.definitions"},

	// Response — finish_reason (OpenAI) and stop_reason (Anthropic) are the same concept
	{"llm.response.finish_reason", "gen_ai.response.finish_reasons"},
	{"llm.response.stop_reason", "gen_ai.response.finish_reasons"},

	// Operation — llm.request.type on LLM spans, traceloop.span.kind on workflow spans (mutually exclusive)
	{"llm.request.type", "gen_ai.operation.name"},

	// Traceloop workflow/entity (agentic)
	{"traceloop.span.kind", "gen_ai.operation.name"},
	{"traceloop.entity.name", "gen_ai.agent.name"},
	{"traceloop.entity.input", "gen_ai.input.messages"},
	{"traceloop.entity.output", "gen_ai.output.messages"},
	{"traceloop.association.properties", "gen_ai.conversation.id"},
}

// --- LangChain / LangGraph ---

var langchainMappings = []Mapping{
	{"lc.metadata.thread_id", "gen_ai.conversation.id"},
	{"langgraph.node.name", "gen_ai.agent.name"},
}

// --- CrewAI ---

var crewaiMappings = []Mapping{
	{"crewai.agent.role", "gen_ai.agent.name"},
	{"crewai.agent.goal", "gen_ai.agent.description"},
	{"crewai.crew.id", "gen_ai.conversation.id"},
}

// --- PydanticAI ---

var pydanticaiMappings = []Mapping{
	{"pydantic_ai.agent.name", "gen_ai.agent.name"},
	{"pydantic_ai.agent.model", "gen_ai.request.model"},
}

// --- Strands (AWS) ---

var strandsMappings = []Mapping{
	{"strands.agent.name", "gen_ai.agent.name"},
	{"strands.agent.model", "gen_ai.request.model"},
	{"strands.tool.name", "gen_ai.tool.name"},
}
