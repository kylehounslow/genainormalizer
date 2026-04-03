package genainormalizer

import (
	"encoding/json"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// msgFormat encapsulates detection and parsing logic for one JSON message serialization format.
// To add support for a new framework, append a new entry to msgFormats — extractAndIndexMessages
// never needs to change.
type msgFormat struct {
	// name is a human-readable label for documentation and debugging.
	name string
	// match returns true when this format applies to the top-level parsed JSON object.
	match func(data map[string]any) bool
	// parse indexes messages into attrs and optionally writes llm.prompt / llm.completion.
	parse func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string)
}

// msgFormats is the ordered registry of known message serialization formats.
// extractAndIndexMessages tries each entry in order; the first match wins.
var msgFormats = []msgFormat{
	{
		// LangChain prompt: {"messages":[[{kwargs:{content,type}}]]}
		name: "langchain_nested_kwargs",
		match: func(data map[string]any) bool {
			msgs, ok := data["messages"].([]any)
			if !ok || len(msgs) == 0 {
				return false
			}
			_, isNested := msgs[0].([]any)
			return isNested
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			outer := data["messages"].([]any)
			inner := outer[0].([]any)
			p.indexMessagesWithKwargs(attrs, inner, messageType)
		},
	},
	{
		// LangChain completion: {"output":{"messages":[{content,type}]}}
		name: "langchain_output_messages",
		match: func(data map[string]any) bool {
			output, ok := data["output"].(map[string]any)
			if !ok {
				return false
			}
			_, ok = output["messages"].([]any)
			return ok
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			output := data["output"].(map[string]any)
			p.indexMessagesDirect(attrs, output["messages"].([]any), messageType)
		},
	},
	{
		// LangChain completion: {"agent_scratchpad":[{content,type}]}
		name: "langchain_agent_scratchpad",
		match: func(data map[string]any) bool {
			_, ok := data["agent_scratchpad"].([]any)
			return ok
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			p.indexMessagesDirect(attrs, data["agent_scratchpad"].([]any), messageType)
		},
	},
	{
		// LangChain/ChatOpenAI completion: {"generations":[[{message:{kwargs:{content,type}}}]]}
		name: "langchain_generations",
		match: func(data map[string]any) bool {
			gens, ok := data["generations"].([]any)
			if !ok || len(gens) == 0 {
				return false
			}
			arr, ok := gens[0].([]any)
			return ok && len(arr) > 0
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			gens := data["generations"].([]any)
			genArr := gens[0].([]any)
			gen, ok := genArr[0].(map[string]any)
			if !ok {
				return
			}
			message, ok := gen["message"].(map[string]any)
			if !ok {
				return
			}
			kwargs, ok := message["kwargs"].(map[string]any)
			if !ok {
				return
			}
			if content, ok := kwargs["content"].(string); ok && content != "" {
				attrs.PutStr("gen_ai."+messageType+".0.content", content)
			}
			if msgType, ok := kwargs["type"].(string); ok {
				attrs.PutStr("gen_ai."+messageType+".0.role", mapLangSmithTypeToRole(msgType))
			}
		},
	},
	{
		// LangGraph prompt/completion: {"messages":[{"role":"user","content":"..."},...]}
		// Detected by: top-level "messages" key whose first element is an object with a "role" field.
		name: "langgraph_flat_messages",
		match: func(data map[string]any) bool {
			msgs, ok := data["messages"].([]any)
			if !ok || len(msgs) == 0 {
				return false
			}
			first, ok := msgs[0].(map[string]any)
			if !ok {
				return false
			}
			_, hasRole := first["role"]
			return hasRole
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			p.indexLangGraphMessages(attrs, data["messages"].([]any), messageType)
		},
	},
	{
		// OpenAI raw response: {"choices":[{"message":{"role":"assistant","content":"..."}}]}
		// Used by wrap_openai LLM spans in LangGraph.
		name: "openai_choices",
		match: func(data map[string]any) bool {
			choices, ok := data["choices"].([]any)
			if !ok || len(choices) == 0 {
				return false
			}
			choice, ok := choices[0].(map[string]any)
			if !ok {
				return false
			}
			_, ok = choice["message"].(map[string]any)
			return ok
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			choices := data["choices"].([]any)
			choice := choices[0].(map[string]any)
			msg := choice["message"].(map[string]any)
			content, ok := msg["content"].(string)
			if !ok || content == "" {
				return
			}
			role, _ := msg["role"].(string)
			attrs.PutStr("gen_ai."+messageType+".0.content", content)
			if role != "" {
				attrs.PutStr("gen_ai."+messageType+".0.role", role)
			}
			// Write plain-text for NR AI monitoring "Response" section.
			attrs.PutStr("gen_ai.completion", content)
			attrs.PutStr("llm.completion", content)
		},
	},
	{
		// LangSmith agent/chain span prompt: {"input":"<string>"}
		// LangGraph wraps its state input as a plain string under "input" on chain spans.
		name: "langsmith_string_input",
		match: func(data map[string]any) bool {
			_, ok := data["input"].(string)
			return ok
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			if messageType != "prompt" {
				return
			}
			str := data["input"].(string)
			if str == "" {
				return
			}
			attrs.PutStr("gen_ai.prompt.0.content", str)
			attrs.PutStr("gen_ai.prompt.0.role", "user")
			attrs.PutStr("gen_ai.prompt", str)
			attrs.PutStr("llm.prompt", str)
		},
	},
	{
		// LangSmith agent/chain span completion: {"output":"<string>"}
		// LangGraph wraps its state output as a plain string under "output" on chain spans.
		// Strings starting with "__" are LangGraph internal routing markers (e.g. "__end__").
		// When a routing marker is detected, the actual response is recovered by promoting
		// the last assistant message already indexed in gen_ai.prompt.N.* attrs.
		name: "langsmith_string_output",
		match: func(data map[string]any) bool {
			_, ok := data["output"].(string)
			return ok
		},
		parse: func(p *genaiNormalizerProcessor, attrs pcommon.Map, data map[string]any, messageType string) {
			if messageType != "completion" {
				return
			}
			str := data["output"].(string)
			if str == "" {
				return
			}
			if strings.HasPrefix(str, "__") {
				// Routing marker — the real response is the last assistant turn in the
				// input conversation, already indexed in gen_ai.prompt.N.* by Pattern 5.
				p.setCompletionFromLastAssistantInPrompt(attrs)
				return
			}
			attrs.PutStr("gen_ai.completion.0.content", str)
			attrs.PutStr("gen_ai.completion.0.role", "assistant")
			attrs.PutStr("gen_ai.completion", str)
			attrs.PutStr("llm.completion", str)
		},
	},
}

// extractAndIndexMessages parses LangSmith JSON and creates indexed message attributes.
// It iterates msgFormats in order; the first matching format wins.
// To support a new serialization format, add an entry to msgFormats above — this function never changes.
func (p *genaiNormalizerProcessor) extractAndIndexMessages(attrs pcommon.Map, jsonStr string, messageType string) {
	if jsonStr == "" {
		return
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return
	}

	for _, format := range msgFormats {
		if format.match(data) {
			format.parse(p, attrs, data, messageType)
			return
		}
	}
}

// indexMessagesWithKwargs indexes messages with {kwargs:{content,type}} structure (LangChain prompt).
func (p *genaiNormalizerProcessor) indexMessagesWithKwargs(attrs pcommon.Map, messages []any, messageType string) {
	for idx, msgRaw := range messages {
		msgMap, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}

		kwargs, ok := msgMap["kwargs"].(map[string]any)
		if !ok {
			continue
		}

		if content, ok := kwargs["content"].(string); ok && content != "" {
			attrs.PutStr("gen_ai."+messageType+"."+strconv.Itoa(idx)+".content", content)
		}

		if msgType, ok := kwargs["type"].(string); ok {
			attrs.PutStr("gen_ai."+messageType+"."+strconv.Itoa(idx)+".role", mapLangSmithTypeToRole(msgType))
		}
	}
}

// indexMessagesDirect indexes messages with {content,type} structure (LangChain completion).
func (p *genaiNormalizerProcessor) indexMessagesDirect(attrs pcommon.Map, messages []any, messageType string) {
	for idx, msgRaw := range messages {
		msgMap, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}

		if content, ok := msgMap["content"].(string); ok && content != "" {
			attrs.PutStr("gen_ai."+messageType+"."+strconv.Itoa(idx)+".content", content)
		}

		if msgType, ok := msgMap["type"].(string); ok {
			attrs.PutStr("gen_ai."+messageType+"."+strconv.Itoa(idx)+".role", mapLangSmithTypeToRole(msgType))
		}
	}
}

// indexLangGraphMessages indexes LangGraph's flat message array format:
// [{"role":"system","content":"..."}, {"role":"user","content":"..."}, ...]
// It also writes llm.prompt / llm.completion as plain-text strings (last user and last assistant

func (p *genaiNormalizerProcessor) indexLangGraphMessages(attrs pcommon.Map, messages []any, messageType string) {
	var lastUserContent, lastAssistantContent string
	idx := 0

	for _, msgRaw := range messages {
		msgMap, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}

		role, _ := msgMap["role"].(string)
		content, _ := msgMap["content"].(string)
		if content == "" {
			continue
		}

		attrs.PutStr("gen_ai."+messageType+"."+strconv.Itoa(idx)+".content", content)
		if role != "" {
			attrs.PutStr("gen_ai."+messageType+"."+strconv.Itoa(idx)+".role", role)
		}
		idx++

		switch role {
		case "user":
			lastUserContent = content
		case "assistant":
			lastAssistantContent = content
		}
	}

	// Write the last user message as llm.prompt and last assistant as llm.completion.
	// These are the plain-text attributes NR AI monitoring reads for "User Input" / "Response".
	if messageType == "prompt" && lastUserContent != "" {
		attrs.PutStr("gen_ai.prompt", lastUserContent)
		attrs.PutStr("llm.prompt", lastUserContent)
	}
	if messageType == "completion" && lastAssistantContent != "" {
		attrs.PutStr("gen_ai.completion", lastAssistantContent)
		attrs.PutStr("llm.completion", lastAssistantContent)
	}
}

// setCompletionFromLastAssistantInPrompt recovers the actual LLM response for spans
// where the output is a LangGraph routing marker (e.g. "__end__").
// It scans the already-indexed gen_ai.prompt.N.role attrs for the highest-indexed
// assistant message and promotes its content to gen_ai.completion / llm.completion.
// This works because LangGraph routing spans receive the full conversation state
// (including the final assistant reply) as their input.
func (p *genaiNormalizerProcessor) setCompletionFromLastAssistantInPrompt(attrs pcommon.Map) {
	lastIdx := -1
	lastContent := ""

	attrs.Range(func(k string, v pcommon.Value) bool {
		if !strings.HasPrefix(k, "gen_ai.prompt.") || !strings.HasSuffix(k, ".role") {
			return true
		}
		if v.Type() != pcommon.ValueTypeStr || v.Str() != "assistant" {
			return true
		}
		// Extract index from "gen_ai.prompt.<idx>.role"
		mid := strings.TrimPrefix(k, "gen_ai.prompt.")
		mid = strings.TrimSuffix(mid, ".role")
		idx, err := strconv.Atoi(mid)
		if err != nil || idx <= lastIdx {
			return true
		}
		contentKey := "gen_ai.prompt." + strconv.Itoa(idx) + ".content"
		if cv, ok := attrs.Get(contentKey); ok && cv.Type() == pcommon.ValueTypeStr && cv.Str() != "" {
			lastIdx = idx
			lastContent = cv.Str()
		}
		return true
	})

	if lastContent == "" {
		return
	}
	attrs.PutStr("gen_ai.completion.0.content", lastContent)
	attrs.PutStr("gen_ai.completion.0.role", "assistant")
	attrs.PutStr("gen_ai.completion", lastContent)
	attrs.PutStr("llm.completion", lastContent)
}

// mapLangSmithTypeToRole maps LangSmith message types to standard roles.
func mapLangSmithTypeToRole(msgType string) string {
	switch msgType {
	case "system":
		return "system"
	case "human":
		return "user"
	case "ai", "AIMessageChunk":
		return "assistant"
	case "tool":
		return "tool"
	default:
		return msgType
	}
}
