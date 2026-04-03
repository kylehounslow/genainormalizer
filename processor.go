package genainormalizer

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
)

const typeStr = "genainormalizer"

type genaiNormalizerProcessor struct {
	next                    consumer.Traces
	lookupTable             map[string]MappingTarget
	removeOrig              bool
	overwrite               bool
	useOpenInferenceProfile bool // Enables OpenInference LangChain response metadata extraction
	useLlamaIndexProfile    bool // Enables LlamaIndex-specific features like gen_ai.system propagation
	useLangSmithProfile     bool // Enables LangSmith-specific features like response_metadata extraction
	useAutoGenProfile       bool // Enables AutoGen-specific features like agent name cleanup
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Profiles:        []string{"openinference", "openllmetry"},
		RemoveOriginals: true,
	}
}

func createTracesProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	c := cfg.(*Config)

	// Check if openinference profile is enabled
	useOpenInference := slices.Contains(c.Profiles, "openinference")
	// Check if llamaindex profile is enabled
	useLlamaIndex := slices.Contains(c.Profiles, "llamaindex")
	// Check if langsmith profile is enabled
	useLangSmith := slices.Contains(c.Profiles, "langsmith")
	// Check if autogen profile is enabled
	useAutoGen := slices.Contains(c.Profiles, "autogen")

	return &genaiNormalizerProcessor{
		next:                    next,
		lookupTable:             BuildLookupTable(c.Profiles),
		removeOrig:              c.RemoveOriginals,
		overwrite:               c.Overwrite,
		useOpenInferenceProfile: useOpenInference,
		useLlamaIndexProfile:    useLlamaIndex,
		useLangSmithProfile:     useLangSmith,
		useAutoGenProfile:       useAutoGen,
	}, nil
}

func (p *genaiNormalizerProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (p *genaiNormalizerProcessor) Start(_ context.Context, _ component.Host) error { return nil }
func (p *genaiNormalizerProcessor) Shutdown(_ context.Context) error                { return nil }

func (p *genaiNormalizerProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	td, err := p.processTraces(ctx, td)
	if err != nil {
		return err
	}
	return p.next.ConsumeTraces(ctx, td)
}

func (p *genaiNormalizerProcessor) processTraces(_ context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rss := td.ResourceSpans()

	// First pass: run attribute mappings on every span.
	for i := 0; i < rss.Len(); i++ {
		ilss := rss.At(i).ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			spans := ilss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				p.normalizeAttributes(spans.At(k).Attributes())
			}
		}
	}

	// Second pass: collect gen_ai.system from normalized spans for trace-wide propagation.
	var genaiSystem string
	if p.useLlamaIndexProfile || p.useLangSmithProfile {
		for i := 0; i < rss.Len() && genaiSystem == ""; i++ {
			ilss := rss.At(i).ScopeSpans()
			for j := 0; j < ilss.Len() && genaiSystem == ""; j++ {
				spans := ilss.At(j).Spans()
				for k := 0; k < spans.Len() && genaiSystem == ""; k++ {
					attrs := spans.At(k).Attributes()
					if v, ok := attrs.Get("gen_ai.system"); ok && v.Str() != "langchain" && v.Str() != "langsmith" {
						genaiSystem = v.Str()
					} else if v, ok := attrs.Get("gen_ai.provider.name"); ok {
						genaiSystem = v.Str()
					}
				}
			}
		}
	}

	// Third pass: profile-specific transforms and gen_ai.system propagation.
	for i := 0; i < rss.Len(); i++ {
		ilss := rss.At(i).ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			scopeName := ilss.At(j).Scope().Name()
			spans := ilss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)

				// Propagate gen_ai.system to spans that don't have a meaningful value.
				if genaiSystem != "" {
					attrs := span.Attributes()
					if existing, ok := attrs.Get("gen_ai.system"); !ok || existing.Str() == "langchain" || existing.Str() == "langsmith" {
						attrs.PutStr("gen_ai.system", genaiSystem)
					}
				}

				// For OpenInference LangChain profile: extract response id, model, and agent name
				if p.useOpenInferenceProfile {
					p.extractOpenInferenceLangChainResponseMetadata(span.Attributes())
					p.extractOpenInferenceLangChainAgentName(span)
				}

				// For LlamaIndex profile: extract current_agent_name from output_value/input_value JSON
				if p.useLlamaIndexProfile {
					p.extractAgentNameFromJSON(span.Attributes())
					p.extractLlamaIndexResponseFromOutputValue(span.Attributes())
				}

				// For LangSmith profile: extract response metadata and normalize messages
				if p.useLangSmithProfile {
					p.extractLangSmithResponseMetadata(span.Attributes())
					p.normalizeLangSmithMessages(span.Attributes())
					p.createInputOutputMessagesFromIndexed(span.Attributes())
					p.extractLangSmithAgentName(span.Attributes())
					p.extractLangSmithTools(span.Attributes())
				}

				// For AutoGen profile: scope-guarded processing per InstrumentationScope.
				if p.useAutoGenProfile {
					switch {
					case strings.HasPrefix(scopeName, "autogen"):
						// Runtime spans: clean up agent name instance suffix.
						p.normalizeAutoGenAttributes(span.Attributes())
					case strings.HasPrefix(scopeName, "opentelemetry.instrumentation.openai"):
						// LLM call spans co-emitted in every AutoGen trace.
						// Extract tool names from prompt indexed attrs and build tool definitions.
						p.extractToolNamesFromPromptIndexed(span.Attributes())
						p.extractToolDefinitionsFromFunctions(span.Attributes())
						// Build gen_ai.input.messages JSON array and gen_ai.completion from indexed attrs.
						p.buildAutoGenInputOutputFromIndexed(span.Attributes())
					}
				}
			}
		}
	}

	// Third pass (AutoGen only): propagate gen_ai.agent.name from runtime spans to LLM spans.
	// Must run after the second pass so agent names are already cleaned up.
	if p.useAutoGenProfile {
		for i := 0; i < rss.Len(); i++ {
			p.propagateAutoGenAgentName(rss.At(i))
		}
	}

	// Fourth pass (AutoGen only): create synthetic tool execution spans from tool call data
	// embedded in openai.chat LLM spans. NR AI monitoring shows tools from individual tool
	// spans (gen_ai.operation.name=execute_tool), not from the LLM span's gen_ai.tool.name.
	if p.useAutoGenProfile {
		for i := 0; i < rss.Len(); i++ {
			p.createAutoGenToolSpans(rss.At(i))
		}
	}

	return td, nil
}

func (p *genaiNormalizerProcessor) normalizeAttributes(attrs pcommon.Map) {
	type rename struct {
		from   string
		target MappingTarget
	}
	var renames []rename

	// First pass: handle simple key mappings
	attrs.Range(func(k string, v pcommon.Value) bool {
		if target, ok := p.lookupTable[k]; ok {
			renames = append(renames, rename{k, target})
		}
		return true
	})

	if len(renames) == 0 {
		return
	}

	for _, r := range renames {
		if val, ok := attrs.Get(r.from); ok {
			// Skip if target attribute already exists and overwrite is disabled
			if _, exists := attrs.Get(r.target.Key); exists && !p.overwrite {
				continue
			}
			if r.target.WrapSlice && val.Type() == pcommon.ValueTypeStr {
				arr := attrs.PutEmptySlice(r.target.Key)
				arr.AppendEmpty().SetStr(val.Str())
			} else if val.Type() == pcommon.ValueTypeStr {
				strVal := val.Str()
				if transformed := TransformValue(r.target.Key, strVal); transformed != strVal {
					attrs.PutStr(r.target.Key, transformed)
				} else {
					val.CopyTo(attrs.PutEmpty(r.target.Key))
				}
			} else {
				val.CopyTo(attrs.PutEmpty(r.target.Key))
			}
			if p.removeOrig {
				attrs.Remove(r.from)
			}
		}
	}

	// Second pass: handle nested attributes dynamically (messages, tools, etc.)
	p.normalizeNestedMessages(attrs)
	p.normalizeNestedAttributesDynamic(attrs)
}

// normalizeNestedAttributesDynamic handles nested attributes with arbitrary depth
// Patterns like: llm.tools.0.tool.json_schema, llm.tools.1.tool.json_schema
// Collects them into arrays: [{...}, {...}]
func (p *genaiNormalizerProcessor) normalizeNestedAttributesDynamic(attrs pcommon.Map) {
	// Regex to match patterns like: prefix.N.suffix where N is a number
	// Example: llm.tools.0.tool.json_schema -> groups: ["llm.tools", "0", "tool.json_schema"]
	nestedPattern := regexp.MustCompile(`^([a-z_\.]+)\.(\d+)\.(.+)$`)

	// Group attributes by their base pattern (prefix + suffix structure)
	type nestedValue struct {
		index int
		path  []string // The nested path after the index
		value pcommon.Value
	}

	// Map: baseKey -> []nestedValue; also track original attribute keys per baseKey so that
	// we only remove source attributes whose baseKey was actually mapped to a target.
	grouped := make(map[string][]nestedValue)
	origKeys := make(map[string][]string) // baseKey → original attribute keys

	attrs.Range(func(k string, v pcommon.Value) bool {
		matches := nestedPattern.FindStringSubmatch(k)
		if len(matches) != 4 {
			return true // Not a nested pattern, skip
		}

		prefix := matches[1]   // e.g., "llm.tools"
		indexStr := matches[2] // e.g., "0"
		suffix := matches[3]   // e.g., "tool.json_schema"

		index, err := strconv.Atoi(indexStr)
		if err != nil {
			return true // Invalid index, skip
		}

		// Build a unique base key for grouping
		baseKey := prefix + "." + suffix

		grouped[baseKey] = append(grouped[baseKey], nestedValue{
			index: index,
			path:  strings.Split(suffix, "."),
			value: v,
		})
		origKeys[baseKey] = append(origKeys[baseKey], k)

		return true
	})

	// Process each group and build nested structures
	var keysToRemove []string
	for baseKey, values := range grouped {
		// Sort by index to maintain order
		sort.Slice(values, func(i, j int) bool {
			return values[i].index < values[j].index
		})

		// Build array of nested objects
		var result []interface{}
		currentIndex := -1
		var currentObj map[string]interface{}

		for _, nv := range values {
			// Start a new object if index changes
			if nv.index != currentIndex {
				if currentObj != nil {
					result = append(result, currentObj)
				}
				currentObj = make(map[string]interface{})
				currentIndex = nv.index
			}

			// Set nested value in currentObj
			setNestedValue(currentObj, nv.path, nv.value)
		}

		// Add last object
		if currentObj != nil {
			result = append(result, currentObj)
		}

		// Determine target attribute name based on source pattern
		targetAttr := mapNestedPatternToTarget(baseKey)
		if targetAttr != "" && len(result) > 0 {
			if resultJSON, err := json.Marshal(result); err == nil {
				attrs.PutStr(targetAttr, string(resultJSON))
			}
			// Only remove source keys that were successfully mapped to a target.
			// Keys with no known target mapping (e.g. llm.request.functions.*) are
			// intentionally left in place for downstream processors to consume.
			if p.removeOrig {
				keysToRemove = append(keysToRemove, origKeys[baseKey]...)
			}
		}
	}

	// Remove original nested attributes that were successfully mapped
	for _, k := range keysToRemove {
		attrs.Remove(k)
	}
}

// setNestedValue sets a value in a nested map structure
// path: ["tool", "json_schema"] -> obj["tool"]["json_schema"] = value
func setNestedValue(obj map[string]interface{}, path []string, value pcommon.Value) {
	if len(path) == 0 {
		return
	}

	// Navigate/create nested structure
	current := obj
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		if _, exists := current[key]; !exists {
			current[key] = make(map[string]interface{})
		}
		if nested, ok := current[key].(map[string]interface{}); ok {
			current = nested
		} else {
			return // Type conflict, skip
		}
	}

	// Set the final value
	finalKey := path[len(path)-1]
	current[finalKey] = convertPcommonValue(value)
}

// convertPcommonValue converts pcommon.Value to Go native type
func convertPcommonValue(v pcommon.Value) interface{} {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str()
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeBool:
		return v.Bool()
	default:
		return nil
	}
}

// mapNestedPatternToTarget maps source patterns to target GenAI attributes
func mapNestedPatternToTarget(baseKey string) string {
	// Map known patterns to GenAI semantic conventions
	patterns := map[string]string{
		"llm.tools.tool.json_schema":    "gen_ai.tool.definitions",
		"llm.tools.tool.name":           "gen_ai.tool.definitions",
		"llm.tools.tool.description":    "gen_ai.tool.definitions",
		"llm.tools.tool.parameters":     "gen_ai.tool.definitions",
		"tool_calls.tool_call.function": "gen_ai.tool.calls",
		"tool_calls.tool_call.id":       "gen_ai.tool.calls",
		"tool_calls.tool_call.type":     "gen_ai.tool.calls",
		// Add more patterns as needed
	}

	// Check for exact matches first
	if target, ok := patterns[baseKey]; ok {
		return target
	}

	// Check for partial matches (prefix matching)
	for pattern, target := range patterns {
		if strings.HasPrefix(baseKey, pattern) || strings.Contains(baseKey, pattern) {
			return target
		}
	}

	return "" // Unknown pattern, don't map
}

// normalizeNestedMessages handles nested OpenInference message attributes
// Pattern: llm.input_messages.N.message.content → gen_ai.prompt
//
//	llm.output_messages.N.message.content → gen_ai.completion
func (p *genaiNormalizerProcessor) normalizeNestedMessages(attrs pcommon.Map) {
	var (
		userPrompt     string
		systemMessage  string
		assistantReply string
		toolName       string
		toolArgs       string
		keysToRemove   []string
	)

	attrs.Range(func(k string, v pcommon.Value) bool {
		// Handle input messages (request)
		if strings.HasPrefix(k, "llm.input_messages.") && strings.Contains(k, ".message.") {
			if strings.HasSuffix(k, ".message.role") && v.Str() == "user" {
				// Found user message, get its content
				contentKey := strings.Replace(k, ".message.role", ".message.content", 1)
				if contentVal, ok := attrs.Get(contentKey); ok {
					userPrompt = contentVal.Str()
				}
			} else if strings.HasSuffix(k, ".message.role") && v.Str() == "system" {
				// Found system message, get its content
				contentKey := strings.Replace(k, ".message.role", ".message.content", 1)
				if contentVal, ok := attrs.Get(contentKey); ok {
					systemMessage = contentVal.Str()
				}
			} else if strings.Contains(k, ".message.tool_calls.0.tool_call.function.name") {
				toolName = v.Str()
			} else if strings.Contains(k, ".message.tool_calls.0.tool_call.function.arguments") {
				toolArgs = v.Str()
			}

			if p.removeOrig {
				keysToRemove = append(keysToRemove, k)
			}
		}

		// Handle output messages (response)
		if strings.HasPrefix(k, "llm.output_messages.") && strings.Contains(k, ".message.") {
			if strings.HasSuffix(k, ".message.role") && v.Str() == "assistant" {
				// Found assistant message, get its content
				contentKey := strings.Replace(k, ".message.role", ".message.content", 1)
				if contentVal, ok := attrs.Get(contentKey); ok {
					assistantReply = contentVal.Str()
				}
			}

			if p.removeOrig {
				keysToRemove = append(keysToRemove, k)
			}
		}

		return true
	})

	// Build official OTel GenAI input messages JSON
	var inputMessages []map[string]interface{}
	if systemMessage != "" {
		inputMessages = append(inputMessages, map[string]interface{}{
			"role":    "system",
			"content": systemMessage,
		})
	}
	if userPrompt != "" {
		inputMessages = append(inputMessages, map[string]interface{}{
			"role":    "user",
			"content": userPrompt,
		})
	}

	// Build official OTel GenAI output messages JSON
	var outputMessages []map[string]interface{}
	if assistantReply != "" {
		outputMessages = append(outputMessages, map[string]interface{}{
			"role":    "assistant",
			"content": assistantReply,
		})
	}

	// Add official OTel GenAI attributes as JSON strings
	if len(inputMessages) > 0 {
		if inputJSON, err := json.Marshal(inputMessages); err == nil {
			attrs.PutStr("gen_ai.input.messages", string(inputJSON))
		}
	}
	if len(outputMessages) > 0 {
		if outputJSON, err := json.Marshal(outputMessages); err == nil {
			attrs.PutStr("gen_ai.output.messages", string(outputJSON))
		}
	}

	// Also add custom simplified attributes for easier querying
	if userPrompt != "" {
		attrs.PutStr("gen_ai.prompt", userPrompt)
		attrs.PutStr("llm.prompt", userPrompt)
	}
	if systemMessage != "" {
		attrs.PutStr("gen_ai.system.message", systemMessage)
		attrs.PutStr("llm.system_message", systemMessage)
	}
	if assistantReply != "" {
		attrs.PutStr("gen_ai.completion", assistantReply)
		attrs.PutStr("llm.completion", assistantReply)
	}
	if toolName != "" {
		attrs.PutStr("gen_ai.tool.name", toolName)
	}
	if toolArgs != "" {
		attrs.PutStr("gen_ai.tool.arguments", toolArgs)
	}

	// Remove original nested attributes if configured
	for _, k := range keysToRemove {
		attrs.Remove(k)
	}
}

// extractAgentNameFromJSON extracts current_agent_name from JSON strings
// in output.value and input.value attributes (LlamaIndex specific)
func (p *genaiNormalizerProcessor) extractAgentNameFromJSON(attrs pcommon.Map) {
	// Check if gen_ai.agent.name already exists
	if _, ok := attrs.Get("gen_ai.agent.name"); ok {
		return // Already set, don't override
	}

	// Try output.value first (note: attribute uses dot, not underscore)
	if val, ok := attrs.Get("output.value"); ok && val.Type() == pcommon.ValueTypeStr {
		if agentName := extractCurrentAgentName(val.Str()); agentName != "" {
			attrs.PutStr("gen_ai.agent.name", agentName)
			return
		}
	}

	// Try input.value as fallback (note: attribute uses dot, not underscore)
	if val, ok := attrs.Get("input.value"); ok && val.Type() == pcommon.ValueTypeStr {
		if agentName := extractCurrentAgentName(val.Str()); agentName != "" {
			attrs.PutStr("gen_ai.agent.name", agentName)
		}
	}
}

// extractCurrentAgentName parses JSON string and extracts current_agent_name field
func extractCurrentAgentName(jsonStr string) string {
	// Handle empty string
	if jsonStr == "" {
		return ""
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// JSON parsing failed - might not be valid JSON
		return ""
	}

	// Look for current_agent_name at top level
	if name, ok := data["current_agent_name"].(string); ok && name != "" {
		return name
	}

	return ""
}

// extractLlamaIndexResponseFromOutputValue extracts gen_ai.response.id, gen_ai.response.model,
// and gen_ai.response.finish_reasons from the output.value JSON emitted by LlamaIndex's
// OpenInference instrumentation. LlamaIndex serializes AgentOutput as JSON in output.value
// which contains a "raw" field with the original OpenAI response.
// Example output.value:
//
//	{"response":{...},"tool_calls":[],"raw":{"id":"chatcmpl-XYZ","model":"gpt-4o-2024-08-06",
//	  "choices":[{"finish_reason":"stop",...}],...},"current_agent_name":"..."}
//
// id and model: only set if not already present (first match wins).
// finish_reason: always overwritten so the last span's value wins (final turn).
func (p *genaiNormalizerProcessor) extractLlamaIndexResponseFromOutputValue(attrs pcommon.Map) {
	val, ok := attrs.Get("output.value")
	if !ok || val.Type() != pcommon.ValueTypeStr {
		return
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(val.Str()), &data); err != nil {
		return
	}

	raw, ok := data["raw"].(map[string]any)
	if !ok {
		return
	}

	// id — only set once (first span wins)
	if _, exists := attrs.Get("gen_ai.response.id"); !exists {
		if id, ok := raw["id"].(string); ok && id != "" {
			attrs.PutStr("gen_ai.response.id", id)
		}
	}

	// model — only set once (first span wins)
	if _, exists := attrs.Get("gen_ai.response.model"); !exists {
		if model, ok := raw["model"].(string); ok && model != "" {
			attrs.PutStr("gen_ai.response.model", model)
		}
	}

	// finish_reason — always overwrite so the last span's value wins
	if choices, ok := raw["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
				attrs.PutStr("gen_ai.response.finish_reasons", reason)
			}
		}
	}
}

// extractOpenInferenceLangChainResponseMetadata extracts gen_ai.response.id and
// gen_ai.response.model from the output.value JSON emitted by
// openinference-instrumentation-langchain on LLM spans.
//
// The instrumentation serialises the full LangChain LLMResult as JSON in output.value.
// The top-level "llm_output" field contains the response metadata directly:
//
//	{"generations":[...],"llm_output":{"id":"chatcmpl-XYZ","model_name":"gpt-4o-2024-08-06",...}}
//
// id and model are only set when not already present (first match wins).
func (p *genaiNormalizerProcessor) extractOpenInferenceLangChainResponseMetadata(attrs pcommon.Map) {
	val, ok := attrs.Get("output.value")
	if !ok || val.Type() != pcommon.ValueTypeStr {
		return
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(val.Str()), &data); err != nil {
		return
	}

	llmOutput, ok := data["llm_output"].(map[string]any)
	if !ok {
		return
	}

	// response id — only set once (first span wins)
	if _, exists := attrs.Get("gen_ai.response.id"); !exists {
		if id, ok := llmOutput["id"].(string); ok && id != "" {
			attrs.PutStr("gen_ai.response.id", id)
		}
	}

	// response model — only set once (first span wins)
	if _, exists := attrs.Get("gen_ai.response.model"); !exists {
		if model, ok := llmOutput["model_name"].(string); ok && model != "" {
			attrs.PutStr("gen_ai.response.model", model)
		}
	}

	// finish_reason — always overwrite so the last span's value wins
	// Path: generations[0][0].generation_info.finish_reason
	if generations, ok := data["generations"].([]any); ok && len(generations) > 0 {
		if row, ok := generations[0].([]any); ok && len(row) > 0 {
			if gen, ok := row[0].(map[string]any); ok {
				if genInfo, ok := gen["generation_info"].(map[string]any); ok {
					if reason, ok := genInfo["finish_reason"].(string); ok && reason != "" {
						attrs.PutStr("gen_ai.response.finish_reasons", reason)
					}
				}
			}
		}
	}
}

// extractOpenInferenceLangChainAgentName sets gen_ai.agent.name on the root LangGraph span.
//
// openinference-instrumentation-langchain does not emit agent.name as a standalone attribute.
// The user-defined agent name comes from compile(name=...) and is reflected in the root span name.
//
// Internal LangGraph pipeline nodes ("agent", "tools", etc.) carry "langgraph_node" in their
// metadata JSON — these are skipped because they represent implementation nodes, not the agent.
// Only the root graph span (which lacks langgraph_node) gets gen_ai.agent.name set.
func (p *genaiNormalizerProcessor) extractOpenInferenceLangChainAgentName(span ptrace.Span) {
	attrs := span.Attributes()

	// Don't override if already set.
	if _, exists := attrs.Get("gen_ai.agent.name"); exists {
		return
	}

	// Skip internal LangGraph node spans — presence of "langgraph_node" in metadata means
	// this is a pipeline node ("agent", "tools", etc.), not the user-defined agent root span.
	if meta, ok := attrs.Get("metadata"); ok && meta.Type() == pcommon.ValueTypeStr {
		var data map[string]any
		if err := json.Unmarshal([]byte(meta.Str()), &data); err == nil {
			if _, hasNode := data["langgraph_node"]; hasNode {
				return
			}
		}
	}

	// Use the span name for invoke_agent spans — the root graph span carries the user-defined
	// name set via compile(name="math_tutor_agent") or equivalent.
	// Skip "LangGraph" — it is the internal default name assigned by LangChain's create_agent()
	// when no explicit name is provided, not a user-defined agent name.
	if opName, ok := attrs.Get("gen_ai.operation.name"); ok && opName.Str() == "invoke_agent" {
		if name := span.Name(); name != "" && name != "LangGraph" {
			attrs.PutStr("gen_ai.agent.name", name)
		}
	}
}

// extractLangSmithResponseMetadata extracts response metadata from LangSmith JSON attributes
// LangSmith embeds response_metadata deep inside gen_ai.prompt and gen_ai.completion JSON
// Example paths:
// - intermediate_steps[N][0].message_log[M].response_metadata.model_name
// - agent_scratchpad[N].response_metadata.model_name
// - messages[N][M].response_metadata.model_name
// - Also extracts "id" field for gen_ai.response.id
func (p *genaiNormalizerProcessor) extractLangSmithResponseMetadata(attrs pcommon.Map) {
	// Try gen_ai.completion first (most likely to have response metadata)
	if val, ok := attrs.Get("gen_ai.completion"); ok && val.Type() == pcommon.ValueTypeStr {
		if p.extractAndSetResponseMetadata(attrs, val.Str()) {
			return // Found and extracted
		}
	}

	// Try gen_ai.prompt as fallback
	if val, ok := attrs.Get("gen_ai.prompt"); ok && val.Type() == pcommon.ValueTypeStr {
		p.extractAndSetResponseMetadata(attrs, val.Str())
	}
}

// extractAndSetResponseMetadata parses JSON and recursively searches for response_metadata
// Returns true if response_metadata was found and extracted
func (p *genaiNormalizerProcessor) extractAndSetResponseMetadata(attrs pcommon.Map, jsonStr string) bool {
	if jsonStr == "" {
		return false
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return false // JSON parsing failed
	}

	// Recursively search for response_metadata anywhere in the JSON structure
	responseMeta, parentObj := findResponseMetadataWithParent(data)
	if responseMeta == nil {
		return false
	}

	// Extract model_name -> gen_ai.response.model
	// Only set if not already present (first match wins for model)
	if modelName, ok := responseMeta["model_name"].(string); ok && modelName != "" {
		if _, exists := attrs.Get("gen_ai.response.model"); !exists {
			attrs.PutStr("gen_ai.response.model", modelName)
		}
	}

	// Extract finish_reason -> gen_ai.response.finish_reasons
	// Always overwrite so the LAST span's finish_reason wins (final result)
	if finishReason, ok := responseMeta["finish_reason"].(string); ok && finishReason != "" {
		attrs.PutStr("gen_ai.response.finish_reasons", finishReason)
	}

	// Extract id from parent object -> gen_ai.response.id
	// The "id" field is typically at the same level as response_metadata
	// Only set if not already present
	if parentObj != nil {
		if responseID, ok := parentObj["id"].(string); ok && responseID != "" {
			if _, exists := attrs.Get("gen_ai.response.id"); !exists {
				attrs.PutStr("gen_ai.response.id", responseID)
			}
		}
	}

	return true
}

// findResponseMetadataWithParent recursively searches for response_metadata and returns both
// the metadata map and its parent object (to extract sibling fields like "id")
func findResponseMetadataWithParent(data any) (map[string]any, map[string]any) {
	switch v := data.(type) {
	case map[string]any:
		// Check if this map has response_metadata key
		if meta, ok := v["response_metadata"].(map[string]any); ok {
			// Skip empty response_metadata objects
			if len(meta) > 0 {
				return meta, v // Return both metadata and parent
			}
		}

		// Recursively search in all map values
		for _, val := range v {
			if meta, parent := findResponseMetadataWithParent(val); meta != nil {
				return meta, parent
			}
		}

	case []any:
		// Recursively search in all array elements
		for _, elem := range v {
			if meta, parent := findResponseMetadataWithParent(elem); meta != nil {
				return meta, parent
			}
		}
	}

	return nil, nil
}

// normalizeLangSmithMessages converts LangSmith's complex JSON message format

func (p *genaiNormalizerProcessor) normalizeLangSmithMessages(attrs pcommon.Map) {
	// Check if gen_ai.prompt exists and contains LangSmith's "messages" format
	promptVal, hasPrompt := attrs.Get("gen_ai.prompt")
	if hasPrompt && promptVal.Type() == pcommon.ValueTypeStr {
		p.extractAndIndexMessages(attrs, promptVal.Str(), "prompt")
	}

	// Check if gen_ai.completion exists and contains message format
	completionVal, hasCompletion := attrs.Get("gen_ai.completion")
	if hasCompletion && completionVal.Type() == pcommon.ValueTypeStr {
		p.extractAndIndexMessages(attrs, completionVal.Str(), "completion")
	}
}

// createInputOutputMessagesFromIndexed creates gen_ai.input.messages and gen_ai.output.messages
// JSON arrays from the indexed message attributes (gen_ai.prompt.N.content, etc.)
// Format: [{"role":"system","content":"..."},{"role":"user","content":"..."}]
// This follows the same pattern as LlamaIndex's normalizeNestedMessages function
func (p *genaiNormalizerProcessor) createInputOutputMessagesFromIndexed(attrs pcommon.Map) {
	// Collect prompt (input) messages
	inputMessages := p.collectIndexedMessages(attrs, "prompt")
	if len(inputMessages) > 0 {
		if inputJSON, err := json.Marshal(inputMessages); err == nil {
			attrs.PutStr("gen_ai.input.messages", string(inputJSON))
		}
	}

	// Collect completion (output) messages
	outputMessages := p.collectIndexedMessages(attrs, "completion")
	if len(outputMessages) > 0 {
		if outputJSON, err := json.Marshal(outputMessages); err == nil {
			attrs.PutStr("gen_ai.output.messages", string(outputJSON))
		}
	}
}

// collectIndexedMessages gathers indexed message attributes into a message array
// Looks for gen_ai.{messageType}.N.content and gen_ai.{messageType}.N.role
func (p *genaiNormalizerProcessor) collectIndexedMessages(attrs pcommon.Map, messageType string) []map[string]interface{} {
	var messages []map[string]interface{}
	messageMap := make(map[int]map[string]string)

	// Collect all indexed message attributes
	attrs.Range(func(k string, v pcommon.Value) bool {
		prefix := "gen_ai." + messageType + "."
		if !strings.HasPrefix(k, prefix) {
			return true
		}

		// Parse: gen_ai.prompt.0.content or gen_ai.prompt.0.role
		suffix := strings.TrimPrefix(k, prefix)
		parts := strings.SplitN(suffix, ".", 2)
		if len(parts) != 2 {
			return true
		}

		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			return true
		}

		field := parts[1] // "content" or "role"

		if messageMap[idx] == nil {
			messageMap[idx] = make(map[string]string)
		}
		messageMap[idx][field] = v.Str()

		return true
	})

	// Convert to sorted array of messages
	if len(messageMap) == 0 {
		return nil
	}

	// Get sorted indices
	indices := make([]int, 0, len(messageMap))
	for idx := range messageMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	// Build message array
	for _, idx := range indices {
		msg := messageMap[idx]
		if content, hasContent := msg["content"]; hasContent && content != "" {
			message := map[string]interface{}{
				"content": content,
			}
			if role, hasRole := msg["role"]; hasRole {
				message["role"] = role
			}
			messages = append(messages, message)
		}
	}

	return messages
}

// extractLangSmithAgentName extracts agent name from LangSmith-specific attributes
// (LangSmith specific - modular approach following LlamaIndex pattern)
func (p *genaiNormalizerProcessor) extractLangSmithAgentName(attrs pcommon.Map) {
	// Check if gen_ai.agent.name already exists
	if _, ok := attrs.Get("gen_ai.agent.name"); ok {
		return // Already set, don't override
	}

	// Skip tool execution spans - they're not agents even if tagged with "agent"
	if spanKind, ok := attrs.Get("langsmith.span.kind"); ok && spanKind.Type() == pcommon.ValueTypeStr {
		if spanKind.Str() == "tool" {
			return // Tool execution span, not an agent
		}
	}

	// Skip internal LangGraph node spans - they have langsmith.metadata.langgraph_node set.
	// Only the root agent span (e.g. "math_tutor_agent") lacks this attribute.
	if _, ok := attrs.Get("langsmith.metadata.langgraph_node"); ok {
		return // Internal LangGraph node span, not the root agent
	}

	// Check langsmith.span.tags for "agent" indicator
	if tags, ok := attrs.Get("langsmith.span.tags"); ok && tags.Type() == pcommon.ValueTypeStr {
		tagsStr := tags.Str()
		// If tags contain "agent", try to extract agent name from trace name
		if containsWord(tagsStr, "agent") {
			if traceName, ok := attrs.Get("langsmith.trace.name"); ok && traceName.Type() == pcommon.ValueTypeStr {
				agentName := extractAgentNameFromTraceName(traceName.Str())
				if agentName != "" {
					attrs.PutStr("gen_ai.agent.name", agentName)
					return
				}
			}
		}
	}

	// Fallback: check gen_ai.prompt and gen_ai.completion for agent-related JSON fields
	for _, attrKey := range []string{"gen_ai.prompt", "gen_ai.completion"} {
		if val, ok := attrs.Get(attrKey); ok && val.Type() == pcommon.ValueTypeStr {
			if agentName := extractAgentNameFromLangSmithJSON(val.Str()); agentName != "" {
				attrs.PutStr("gen_ai.agent.name", agentName)
				return
			}
		}
	}
}

// extractAgentNameFromTraceName extracts user-defined agent names from langsmith.trace.name
// Only accepts names that look like user-defined agents, not internal LangChain components
func extractAgentNameFromTraceName(traceName string) string {
	// Examples from actual logs:
	// ✓ ACCEPT: "math_tutor_agent" → user-defined agent
	// ✗ REJECT: "RunnableAssign<agent_scratchpad>" → internal runnable
	// ✗ REJECT: "ToolsAgentOutputParser" → internal parser
	// ✗ REJECT: "ChatPromptTemplate" → internal template

	// Reject names with angle brackets (internal runnables)
	if strings.Contains(traceName, "<") || strings.Contains(traceName, ">") {
		return ""
	}

	// Reject internal LangChain class names (camelCase patterns)
	if isInternalLangChainComponent(traceName) {
		return ""
	}

	// Accept simple names that look user-defined (lowercase with underscores/hyphens)
	if isUserDefinedAgentName(traceName) {
		return traceName
	}

	return ""
}

// isInternalLangChainComponent checks if a name matches internal LangChain patterns
func isInternalLangChainComponent(name string) bool {
	// Internal component patterns
	internalPrefixes := []string{"Runnable", "Chat", "Tools", "Agent", "Output"}
	internalSuffixes := []string{"Parser", "Template", "Chain", "Executor", "Lambda"}
	internalNames := []string{"agent_scratchpad", "output_parser", "input", "output"}

	// Check for camelCase class names (uppercase letter present)
	hasUpperCase := false
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			hasUpperCase = true
			break
		}
	}

	// If contains uppercase, check if it matches internal patterns
	if hasUpperCase {
		for _, prefix := range internalPrefixes {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
		for _, suffix := range internalSuffixes {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
	}

	// Check for generic internal names
	lowerName := strings.ToLower(name)
	for _, internal := range internalNames {
		if lowerName == internal {
			return true
		}
	}

	return false
}

// isUserDefinedAgentName checks if a name looks like a user-defined agent
func isUserDefinedAgentName(name string) bool {
	// User-defined agent names typically:
	// - Are lowercase or contain underscores/hyphens
	// - Don't match internal patterns
	// - Are reasonable length (not too short, not too long)

	if name == "" || len(name) < 3 || len(name) > 100 {
		return false
	}

	// Check for simple identifier pattern: lowercase letters, numbers, underscores, hyphens
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}

	return true
}

// extractAgentNameFromLangSmithJSON searches for agent-related fields in JSON
func extractAgentNameFromLangSmithJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return ""
	}

	// Look for agent-related fields at top level
	for _, key := range []string{"agent_name", "current_agent_name", "agent"} {
		if name, ok := data[key].(string); ok && name != "" {
			return name
		}
	}

	return ""
}


// (LangSmith specific - modular approach following LlamaIndex pattern)
func (p *genaiNormalizerProcessor) extractLangSmithTools(attrs pcommon.Map) {
	// Priority 1: Tool execution spans (langsmith.span.kind == "tool")
	// Extract tool name from langsmith.trace.name
	if spanKind, ok := attrs.Get("langsmith.span.kind"); ok && spanKind.Type() == pcommon.ValueTypeStr {
		if spanKind.Str() == "tool" {
			if traceName, ok := attrs.Get("langsmith.trace.name"); ok && traceName.Type() == pcommon.ValueTypeStr {
				toolName := traceName.Str()
				if toolName != "" && isValidToolName(toolName) {
					attrs.PutStr("gen_ai.tool.name", toolName)
					return // Tool execution spans only have one tool
				}
			}
		}
	}

	// Priority 2: Tool calls in JSON (prompt/completion)
	// Extract from tool_calls in JSON and set gen_ai.tool.name to first tool
	toolsSet := make(map[string]bool) // Use map to deduplicate tool names

	// Search in gen_ai.prompt and gen_ai.completion
	for _, attrKey := range []string{"gen_ai.prompt", "gen_ai.completion"} {
		if val, ok := attrs.Get(attrKey); ok && val.Type() == pcommon.ValueTypeStr {
			tools := extractToolNamesFromJSON(val.Str())
			for _, tool := range tools {
				if isValidToolName(tool) {
					toolsSet[tool] = true
				}
			}
		}
	}

	
	if len(toolsSet) > 0 {
		var tools []string
		for tool := range toolsSet {
			tools = append(tools, tool)
		}
		sort.Strings(tools)

		// Set the primary tool name (first in sorted list for consistency)
		attrs.PutStr("gen_ai.tool.name", tools[0])

		// If multiple tools, also store full list as JSON for reference
		if len(tools) > 1 {
			toolsJSON, _ := json.Marshal(tools)
			attrs.PutStr("gen_ai.tool.definitions", string(toolsJSON))
		}
	}
}

// isValidToolName checks if a string is a valid tool name (filters out garbage)
func isValidToolName(name string) bool {
	if name == "" || len(name) < 2 || len(name) > 100 {
		return false
	}
	// Tool names typically use snake_case or camelCase
	// Accept: letters, numbers, underscores, hyphens, periods
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// extractToolNamesFromJSON parses JSON and extracts tool names from various structures
func extractToolNamesFromJSON(jsonStr string) []string {
	var tools []string

	if jsonStr == "" {
		return tools
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return tools
	}

	// Recursively search for tool_calls, tools, and function.name patterns
	tools = append(tools, searchToolsInMap(data)...)

	return tools
}

// searchToolsInMap recursively searches for tool names in a map structure
func searchToolsInMap(data map[string]any) []string {
	var tools []string

	for key, value := range data {
		switch v := value.(type) {
		case string:
			// Direct tool name field
			if key == "tool" || key == "name" {
				// Only consider it a tool name if parent context suggests it's a tool
				if v != "" && !strings.Contains(v, " ") && len(v) < 100 {
					tools = append(tools, v)
				}
			}

		case []any:
			// Array of tool calls or messages
			if key == "tool_calls" || key == "tools" {
				for _, item := range v {
					if toolMap, ok := item.(map[string]any); ok {
						// Format 1: {"name": "add_numbers", ...}
						if name, ok := toolMap["name"].(string); ok && name != "" {
							tools = append(tools, name)
						}
						// Format 2: {"function": {"name": "add_numbers"}, ...}
						if fn, ok := toolMap["function"].(map[string]any); ok {
							if name, ok := fn["name"].(string); ok && name != "" {
								tools = append(tools, name)
							}
						}
						// Recursively search nested structures
						tools = append(tools, searchToolsInMap(toolMap)...)
					}
				}
			} else {
				// Search in other arrays
				for _, item := range v {
					if itemMap, ok := item.(map[string]any); ok {
						tools = append(tools, searchToolsInMap(itemMap)...)
					}
				}
			}

		case map[string]any:
			// Recursively search nested maps
			if key == "additional_kwargs" || key == "kwargs" || key == "message" {
				tools = append(tools, searchToolsInMap(v)...)
			} else if key == "function" {
				// Function object typically has "name" field
				if name, ok := v["name"].(string); ok && name != "" {
					tools = append(tools, name)
				}
				tools = append(tools, searchToolsInMap(v)...)
			} else {
				tools = append(tools, searchToolsInMap(v)...)
			}
		}
	}

	return tools
}

// containsWord checks if a string contains a specific word (case-insensitive, comma/space delimited)
func containsWord(s, word string) bool {
	lower := strings.ToLower(s)
	lowerWord := strings.ToLower(word)

	// Split by common delimiters
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';' || r == '\t'
	})

	for _, w := range words {
		if w == lowerWord {
			return true
		}
	}
	return false
}

// autogenInstanceSuffix matches the agent instance suffix AutoGen appends to agent names.
// Example: "math_tutor.(default)-A" → strip to "math_tutor"
// Pattern: dot + parenthesised namespace + hyphen + alphanumeric instance id at end of string.
var autogenInstanceSuffix = regexp.MustCompile(`\.\([^)]*\)-[A-Za-z0-9]+$`)

// extractToolNamesFromPromptIndexed scans the span's attributes for tool call names embedded
// in the indexed prompt/completion messages emitted by opentelemetry-instrumentation-openai:
//
//	gen_ai.prompt.N.tool_calls.M.name     →  "add_numbers"  (tool calls echoed in follow-up turn)
//	gen_ai.completion.N.tool_calls.M.name →  "add_numbers"  (tool calls in the LLM response)
//
// Unique names are collected, sorted, and written to:
//   - gen_ai.tool.name        — first sorted tool name (plain string), same as LangSmith/LlamaIndex
//   - gen_ai.tool.definitions — JSON string array of ALL called tool names, e.g. ["add_numbers","subtract_numbers"]
//     This matches LangSmith's multi-tool pattern so NR AI monitoring shows all tools.
//
// Does nothing if gen_ai.tool.name is already set.
func (p *genaiNormalizerProcessor) extractToolNamesFromPromptIndexed(attrs pcommon.Map) {
	if _, ok := attrs.Get("gen_ai.tool.name"); ok {
		return // Already set, don't override.
	}

	toolsSet := make(map[string]bool)
	attrs.Range(func(k string, v pcommon.Value) bool {
		if v.Type() != pcommon.ValueTypeStr {
			return true
		}
		// Match: gen_ai.prompt.<N>.tool_calls.<M>.name
		//        gen_ai.completion.<N>.tool_calls.<M>.name
		if (strings.HasPrefix(k, "gen_ai.prompt.") || strings.HasPrefix(k, "gen_ai.completion.")) &&
			strings.Contains(k, ".tool_calls.") &&
			strings.HasSuffix(k, ".name") {
			if name := v.Str(); name != "" {
				toolsSet[name] = true
			}
		}
		return true
	})

	if len(toolsSet) == 0 {
		return
	}

	tools := make([]string, 0, len(toolsSet))
	for name := range toolsSet {
		tools = append(tools, name)
	}
	sort.Strings(tools)

	// Primary tool name — plain string, same pattern as LangSmith and LlamaIndex.
	attrs.PutStr("gen_ai.tool.name", tools[0])

	// All called tool names as a JSON string array — matches LangSmith's gen_ai.tool.definitions
	// format so NR AI monitoring can display every tool that was called in this turn.
	if toolsJSON, err := json.Marshal(tools); err == nil {
		attrs.PutStr("gen_ai.tool.definitions", string(toolsJSON))
	}
}

// extractToolDefinitionsFromFunctions builds a structured gen_ai.tool.schemas JSON array
// from the indexed tool definition attributes emitted by opentelemetry-instrumentation-openai:
//
//	llm.request.functions.N.name         →  "add_numbers"
//	llm.request.functions.N.description  →  "Add two numbers together"
//	llm.request.functions.N.parameters   →  "{...JSON schema...}"
//
// All three fields for each index N are combined into one object per tool.
// Written to gen_ai.tool.schemas (not gen_ai.tool.definitions) so it does not overwrite
// the called-tool-names array set by extractToolNamesFromPromptIndexed.
func (p *genaiNormalizerProcessor) extractToolDefinitionsFromFunctions(attrs pcommon.Map) {
	type toolDef struct {
		Name        string
		Description string
		Parameters  string
	}
	toolMap := make(map[int]*toolDef)
	var keysToRemove []string

	attrs.Range(func(k string, v pcommon.Value) bool {
		const prefix = "llm.request.functions."
		if !strings.HasPrefix(k, prefix) {
			return true
		}
		rest := strings.TrimPrefix(k, prefix)
		dotIdx := strings.Index(rest, ".")
		if dotIdx < 0 {
			return true
		}
		idx, err := strconv.Atoi(rest[:dotIdx])
		if err != nil {
			return true
		}
		field := rest[dotIdx+1:]

		if toolMap[idx] == nil {
			toolMap[idx] = &toolDef{}
		}
		switch field {
		case "name":
			toolMap[idx].Name = v.Str()
		case "description":
			toolMap[idx].Description = v.Str()
		case "parameters":
			toolMap[idx].Parameters = v.Str()
		}
		if p.removeOrig {
			keysToRemove = append(keysToRemove, k)
		}
		return true
	})

	if len(toolMap) == 0 {
		return
	}

	indices := make([]int, 0, len(toolMap))
	for idx := range toolMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var tools []map[string]any
	for _, idx := range indices {
		t := toolMap[idx]
		entry := make(map[string]any)
		if t.Name != "" {
			entry["name"] = t.Name
		}
		if t.Description != "" {
			entry["description"] = t.Description
		}
		if t.Parameters != "" {
			// parameters is a JSON string — embed as raw JSON so it's not double-encoded.
			var params any
			if err := json.Unmarshal([]byte(t.Parameters), &params); err == nil {
				entry["parameters"] = params
			} else {
				entry["parameters"] = t.Parameters
			}
		}
		if len(entry) > 0 {
			tools = append(tools, entry)
		}
	}

	if len(tools) > 0 {
		if toolsJSON, err := json.Marshal(tools); err == nil {
			attrs.PutStr("gen_ai.tool.schemas", string(toolsJSON))
		}
	}

	for _, k := range keysToRemove {
		attrs.Remove(k)
	}
}

// normalizeAutoGenAttributes post-processes attributes after the lookup-table rename
// (messaging.* → gen_ai.*) has already run. Called only for spans whose InstrumentationScope
// starts with "autogen" (guarded at the call site in ConsumeTraces).
//
//   - gen_ai.agent.name: strips the instance suffix (e.g. ".(default)-A"), leaving the bare
//     agent type name. Removes the attribute when the destination was empty (e.g. "autogen ack"
//     acknowledgement spans have messaging.destination = "").
func (p *genaiNormalizerProcessor) normalizeAutoGenAttributes(attrs pcommon.Map) {
	agentName, ok := attrs.Get("gen_ai.agent.name")
	if !ok || agentName.Type() != pcommon.ValueTypeStr {
		return
	}
	name := agentName.Str()
	if name == "" {
		// Empty destination on ack spans — remove the placeholder.
		attrs.Remove("gen_ai.agent.name")
		return
	}
	// Strip instance suffix: "math_tutor.(default)-A" → "math_tutor"
	if cleaned := autogenInstanceSuffix.ReplaceAllString(name, ""); cleaned != name {
		attrs.PutStr("gen_ai.agent.name", cleaned)
	}
}

// buildAutoGenInputOutputFromIndexed assembles gen_ai.input.messages (JSON array) and
// gen_ai.completion (string) from the indexed span attributes emitted by
// opentelemetry-instrumentation-openai for AutoGen traces.
//
// Input: gen_ai.prompt.N.{role,content,tool_call_id,tool_calls.M.{id,name,arguments}}
// Output:
//
//	gen_ai.input.messages = '[{"role":"user","content":"..."},{"role":"assistant","tool_calls":[...]},...]'
//	gen_ai.completion     = "The result of..." (only when finish_reason is "stop")
//
// Does nothing if gen_ai.input.messages is already set.
func (p *genaiNormalizerProcessor) buildAutoGenInputOutputFromIndexed(attrs pcommon.Map) {
	if _, ok := attrs.Get("gen_ai.input.messages"); ok {
		return
	}

	// --- collect prompt (input) messages ---
	type rawToolCall struct {
		id        string
		name      string
		arguments string
	}
	type rawMsg struct {
		role      string
		content   string
		tcID      string // tool_call_id on tool-result messages
		toolCalls map[int]*rawToolCall
	}

	promptRaw := make(map[int]*rawMsg)

	attrs.Range(func(k string, v pcommon.Value) bool {
		if !strings.HasPrefix(k, "gen_ai.prompt.") || v.Type() != pcommon.ValueTypeStr {
			return true
		}
		suffix := strings.TrimPrefix(k, "gen_ai.prompt.")
		dotIdx := strings.IndexByte(suffix, '.')
		if dotIdx < 0 {
			return true
		}
		idx, err := strconv.Atoi(suffix[:dotIdx])
		if err != nil {
			return true
		}
		if promptRaw[idx] == nil {
			promptRaw[idx] = &rawMsg{toolCalls: make(map[int]*rawToolCall)}
		}
		field := suffix[dotIdx+1:]
		switch field {
		case "role":
			promptRaw[idx].role = v.Str()
		case "content":
			promptRaw[idx].content = v.Str()
		case "tool_call_id":
			promptRaw[idx].tcID = v.Str()
		default:
			if strings.HasPrefix(field, "tool_calls.") {
				// tool_calls.<M>.{id,name,arguments}
				rest := strings.TrimPrefix(field, "tool_calls.")
				dot2 := strings.IndexByte(rest, '.')
				if dot2 < 0 {
					return true
				}
				tcIdx, err := strconv.Atoi(rest[:dot2])
				if err != nil {
					return true
				}
				if promptRaw[idx].toolCalls[tcIdx] == nil {
					promptRaw[idx].toolCalls[tcIdx] = &rawToolCall{}
				}
				switch rest[dot2+1:] {
				case "id":
					promptRaw[idx].toolCalls[tcIdx].id = v.Str()
				case "name":
					promptRaw[idx].toolCalls[tcIdx].name = v.Str()
				case "arguments":
					promptRaw[idx].toolCalls[tcIdx].arguments = v.Str()
				}
			}
		}
		return true
	})

	if len(promptRaw) > 0 {
		// sort by index and build message objects
		promptIdxs := make([]int, 0, len(promptRaw))
		for i := range promptRaw {
			promptIdxs = append(promptIdxs, i)
		}
		sort.Ints(promptIdxs)

		var messages []map[string]interface{}
		for _, i := range promptIdxs {
			raw := promptRaw[i]
			msg := map[string]interface{}{"role": raw.role}

			if len(raw.toolCalls) > 0 {
				// assistant message carrying tool call requests
				tcIdxs := make([]int, 0, len(raw.toolCalls))
				for ti := range raw.toolCalls {
					tcIdxs = append(tcIdxs, ti)
				}
				sort.Ints(tcIdxs)
				var tcs []map[string]interface{}
				for _, ti := range tcIdxs {
					tc := raw.toolCalls[ti]
					tcObj := map[string]interface{}{"id": tc.id, "name": tc.name}
					var parsedArgs interface{}
					if json.Unmarshal([]byte(tc.arguments), &parsedArgs) == nil {
						tcObj["arguments"] = parsedArgs
					} else {
						tcObj["arguments"] = tc.arguments
					}
					tcs = append(tcs, tcObj)
				}
				msg["tool_calls"] = tcs
			} else {
				msg["content"] = raw.content
			}
			if raw.tcID != "" {
				msg["tool_call_id"] = raw.tcID
			}
			messages = append(messages, msg)
		}

		if inputJSON, err := json.Marshal(messages); err == nil {
			attrs.PutStr("gen_ai.input.messages", string(inputJSON))
		}

		// Also set gen_ai.prompt as a flat string (last user message content) so NR
		// displays the correct value in the "User input" field.
		lastUserContent := ""
		for _, i := range promptIdxs {
			raw := promptRaw[i]
			if raw.role == "user" && raw.content != "" {
				lastUserContent = raw.content
			}
		}
		if lastUserContent != "" {
			attrs.PutStr("gen_ai.prompt", lastUserContent)
			attrs.PutStr("llm.prompt", lastUserContent) // NR AI monitoring reads llm.prompt for "User input" display
		}
	}

	// --- build gen_ai.completion from the final assistant text response ---
	if _, ok := attrs.Get("gen_ai.completion"); ok {
		return
	}
	finishReason := ""
	if fr, ok := attrs.Get("gen_ai.completion.0.finish_reason"); ok {
		finishReason = fr.Str()
	}
	// Only set completion text when the model gave a final answer (not a tool-call turn).
	if finishReason == "stop" || finishReason == "end_turn" {
		if content, ok := attrs.Get("gen_ai.completion.0.content"); ok && content.Str() != "" {
			completionText := content.Str()
			attrs.PutStr("gen_ai.completion", completionText)
			attrs.PutStr("llm.completion", completionText) // NR AI monitoring reads llm.completion for "Response" display
			// gen_ai.output.messages JSON array (mirrors LlamaIndex pattern)
			outputMsg := []map[string]interface{}{{"role": "assistant", "content": completionText}}
			if outputJSON, err := json.Marshal(outputMsg); err == nil {
				attrs.PutStr("gen_ai.output.messages", string(outputJSON))
			}
		}
	}
}

// propagateAutoGenAgentName copies gen_ai.agent.name from autogen runtime spans to the
// co-emitted OpenAI instrumentation spans within the same ResourceSpans batch.
//
// AutoGen emits two InstrumentationScopes per trace:
//   - "autogen SingleThreadedAgentRuntime" — runtime spans (send/create/ack) that carry gen_ai.agent.name
//   - "opentelemetry.instrumentation.openai.v1" — LLM call spans that lack gen_ai.agent.name
//

func (p *genaiNormalizerProcessor) propagateAutoGenAgentName(rs ptrace.ResourceSpans) {
	ilss := rs.ScopeSpans()

	// Collect the first non-empty agent name from autogen runtime scopes.
	var agentName string
	for j := 0; j < ilss.Len(); j++ {
		if !strings.HasPrefix(ilss.At(j).Scope().Name(), "autogen") {
			continue
		}
		spans := ilss.At(j).Spans()
		for k := 0; k < spans.Len(); k++ {
			if v, ok := spans.At(k).Attributes().Get("gen_ai.agent.name"); ok {
				if name := v.Str(); name != "" {
					agentName = name
					break
				}
			}
		}
		if agentName != "" {
			break
		}
	}

	if agentName == "" {
		return
	}

	// Set gen_ai.agent.name on all OpenAI instrumentation spans that don't already have it.
	for j := 0; j < ilss.Len(); j++ {
		if !strings.HasPrefix(ilss.At(j).Scope().Name(), "opentelemetry.instrumentation.openai") {
			continue
		}
		spans := ilss.At(j).Spans()
		for k := 0; k < spans.Len(); k++ {
			attrs := spans.At(k).Attributes()
			if _, ok := attrs.Get("gen_ai.agent.name"); !ok {
				attrs.PutStr("gen_ai.agent.name", agentName)
			}
		}
	}
}

// createAutoGenToolSpans creates synthetic tool execution spans from tool call data
// embedded in openai.chat LLM spans. NR AI monitoring displays tools from dedicated
// spans with gen_ai.operation.name="execute_tool" + gen_ai.tool.name, not from the
// LLM span's gen_ai.tool.name string. One span is created per unique tool call ID.
func (p *genaiNormalizerProcessor) createAutoGenToolSpans(rs ptrace.ResourceSpans) {
	type toolCall struct {
		id           string
		name         string
		arguments    string
		parentSpanID pcommon.SpanID
		traceID      pcommon.TraceID
		startTime    pcommon.Timestamp
		endTime      pcommon.Timestamp
		system       string
		agentName    string
		model        string
	}

	seenCallIDs := make(map[string]bool)
	var toolCalls []toolCall
	var openaiScopeSpans ptrace.ScopeSpans
	foundScope := false

	ilss := rs.ScopeSpans()
	for j := 0; j < ilss.Len(); j++ {
		ils := ilss.At(j)
		if !strings.HasPrefix(ils.Scope().Name(), "opentelemetry.instrumentation.openai") {
			continue
		}
		if !foundScope {
			openaiScopeSpans = ils
			foundScope = true
		}

		spans := ils.Spans()
		for k := 0; k < spans.Len(); k++ {
			span := spans.At(k)
			attrs := span.Attributes()

			// Only scan gen_ai.completion.N.tool_calls.M.* — these are tool calls
			// decided by the model in this span. Skipping gen_ai.prompt.* avoids
			// duplicates from echoed conversation history in subsequent spans.
			type rawTC struct {
				id        string
				name      string
				arguments string
			}
			tcByIdx := make(map[string]*rawTC) // key: "N.M"
			attrs.Range(func(key string, v pcommon.Value) bool {
				if !strings.HasPrefix(key, "gen_ai.completion.") || !strings.Contains(key, ".tool_calls.") {
					return true
				}
				rest := strings.TrimPrefix(key, "gen_ai.completion.")
				dot1 := strings.IndexByte(rest, '.')
				if dot1 < 0 {
					return true
				}
				n := rest[:dot1]
				rest = rest[dot1+1:]
				if !strings.HasPrefix(rest, "tool_calls.") {
					return true
				}
				rest = strings.TrimPrefix(rest, "tool_calls.")
				dot2 := strings.IndexByte(rest, '.')
				if dot2 < 0 {
					return true
				}
				m, field := rest[:dot2], rest[dot2+1:]
				tcKey := n + "." + m
				if tcByIdx[tcKey] == nil {
					tcByIdx[tcKey] = &rawTC{}
				}
				switch field {
				case "id":
					tcByIdx[tcKey].id = v.Str()
				case "name":
					tcByIdx[tcKey].name = v.Str()
				case "arguments":
					tcByIdx[tcKey].arguments = v.Str()
				}
				return true
			})

			if len(tcByIdx) == 0 {
				continue
			}

			system, agentName, model := "", "", ""
			if v, ok := attrs.Get("gen_ai.system"); ok {
				system = v.Str()
			}
			if v, ok := attrs.Get("gen_ai.agent.name"); ok {
				agentName = v.Str()
			}
			if v, ok := attrs.Get("gen_ai.request.model"); ok {
				model = v.Str()
			}

			for _, tc := range tcByIdx {
				if tc.name == "" {
					continue
				}
				// Deduplicate by call ID — same call may echo in later spans' prompt history.
				if tc.id != "" {
					if seenCallIDs[tc.id] {
						continue
					}
					seenCallIDs[tc.id] = true
				}
				toolCalls = append(toolCalls, toolCall{
					id:           tc.id,
					name:         tc.name,
					arguments:    tc.arguments,
					parentSpanID: span.SpanID(),
					traceID:      span.TraceID(),
					startTime:    span.StartTimestamp(),
					endTime:      span.EndTimestamp(),
					system:       system,
					agentName:    agentName,
					model:        model,
				})
			}
		}
	}

	if len(toolCalls) == 0 || !foundScope {
		return
	}

	for _, tc := range toolCalls {
		newSpan := openaiScopeSpans.Spans().AppendEmpty()
		newSpan.SetTraceID(tc.traceID)
		newSpan.SetParentSpanID(tc.parentSpanID)
		newSpan.SetName("execute_tool " + tc.name)
		newSpan.SetKind(ptrace.SpanKindInternal)
		newSpan.SetStartTimestamp(tc.startTime)
		newSpan.SetEndTimestamp(tc.endTime)
		newSpan.Status().SetCode(ptrace.StatusCodeUnset)

		// Deterministic span ID from call ID (XOR bytes into 8-byte array).
		seed := tc.id
		if seed == "" {
			seed = tc.name
		}
		var spanID pcommon.SpanID
		for i, b := range []byte(seed) {
			spanID[i%8] ^= b
		}
		if spanID == (pcommon.SpanID{}) {
			spanID[0] = 1
		}
		newSpan.SetSpanID(spanID)

		a := newSpan.Attributes()
		a.PutStr("gen_ai.operation.name", "execute_tool")
		a.PutStr("gen_ai.tool.name", tc.name)
		if tc.id != "" {
			a.PutStr("gen_ai.tool.call.id", tc.id)
		}
		if tc.arguments != "" {
			a.PutStr("gen_ai.tool.arguments", tc.arguments)
		}
		if tc.system != "" {
			a.PutStr("gen_ai.system", tc.system)
		}
		if tc.agentName != "" {
			a.PutStr("gen_ai.agent.name", tc.agentName)
		}
		if tc.model != "" {
			a.PutStr("gen_ai.request.model", tc.model)
		}
	}
}

