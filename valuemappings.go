package genainormalizer

import "strings"

// ValueMapping defines source→target value transforms for a specific attribute.
// After key rename, if the target key has a value map, the value is also transformed.
type ValueMapping map[string]string

// ValueMappings returns value maps keyed by TARGET attribute name.
// Only attributes that need value transformation (not just key rename) are listed.
func ValueMappings() map[string]ValueMapping {
	return map[string]ValueMapping{
		"gen_ai.operation.name": operationNameValues,
	}
}

// OpenInference span.kind → gen_ai.operation.name
// OpenLLMetry traceloop.span.kind → gen_ai.operation.name
// OpenLLMetry llm.request.type → gen_ai.operation.name
//
// Sources:
//   OpenInference: https://github.com/Arize-ai/openinference/blob/main/spec/semantic_conventions.md#span-kinds
//   OpenLLMetry:   traceloop.span.kind values from semconv_ai/__init__.py TraceloopSpanKindValues
//   OpenLLMetry:   llm.request.type values from semconv_ai/__init__.py LLMRequestTypeValues
//   OTel GenAI:    https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/
var operationNameValues = ValueMapping{
	// OpenInference span kinds (uppercase)
	"LLM":       "chat",
	"EMBEDDING": "embeddings",
	"CHAIN":     "invoke_agent", // Chain is the closest to agent invocation
	"RETRIEVER": "retrieval",
	"RERANKER":  "retrieval",   // No dedicated rerank op; closest is retrieval
	"TOOL":      "execute_tool",
	"AGENT":     "invoke_agent",
	"GUARDRAIL": "guardrail",   // No OTel equivalent; pass through as custom value
	"EVALUATOR": "evaluate",    // No OTel equivalent; pass through as custom value
	"PROMPT":    "text_completion",

	// OpenLLMetry traceloop.span.kind (lowercase)
	"workflow": "invoke_agent",
	"task":     "invoke_agent",
	"agent":    "invoke_agent",
	"tool":     "execute_tool",

	// OpenLLMetry llm.request.type (lowercase)
	"completion": "text_completion",
	"chat":       "chat",
	"rerank":     "retrieval",
	"embedding":  "embeddings",
}

// TransformValue applies value mapping for a given target attribute key.
// Returns the transformed value, or the original if no mapping exists.
// Lookup is case-insensitive on the source value.
func TransformValue(targetKey string, value string) string {
	mappings := ValueMappings()
	vm, ok := mappings[targetKey]
	if !ok {
		return value
	}
	// Try exact match first, then lowercase
	if mapped, ok := vm[value]; ok {
		return mapped
	}
	if mapped, ok := vm[strings.ToLower(value)]; ok {
		return mapped
	}
	if mapped, ok := vm[strings.ToUpper(value)]; ok {
		return mapped
	}
	return value
}
