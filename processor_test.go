package genainormalizer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"
)

func TestNormalizeOpenInference(t *testing.T) {
	cfg := &Config{Profiles: []string{"openinference"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutInt("llm.token_count.prompt", 100)
	span.Attributes().PutInt("llm.token_count.completion", 200)
	span.Attributes().PutStr("llm.model_name", "claude-sonnet-4")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "gen_ai.usage.input_tokens", 100)
	assertAttrInt(t, out, "gen_ai.usage.output_tokens", 200)
	assertAttrStr(t, out, "gen_ai.request.model", "claude-sonnet-4")

	if _, ok := out.Get("llm.token_count.prompt"); ok {
		t.Error("expected llm.token_count.prompt to be removed")
	}
}

func TestNormalizeOpenLLMetry(t *testing.T) {
	cfg := &Config{Profiles: []string{"openllmetry"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutInt("llm.usage.prompt_tokens", 150)
	span.Attributes().PutStr("llm.request.model", "gpt-4o")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "gen_ai.usage.input_tokens", 150)
	assertAttrStr(t, out, "gen_ai.request.model", "gpt-4o")
}

func TestOpenLLMetryOnlyIgnoresOpenInference(t *testing.T) {
	cfg := &Config{Profiles: []string{"openllmetry"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutInt("llm.token_count.prompt", 100)
	span.Attributes().PutStr("llm.model_name", "claude-sonnet-4")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "llm.token_count.prompt", 100)
	assertAttrStr(t, out, "llm.model_name", "claude-sonnet-4")
	if _, ok := out.Get("gen_ai.usage.input_tokens"); ok {
		t.Error("OpenInference attr should not be mapped when only openllmetry profile is enabled")
	}
}

func TestOpenInferenceOnlyIgnoresOpenLLMetry(t *testing.T) {
	cfg := &Config{Profiles: []string{"openinference"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutInt("llm.usage.prompt_tokens", 200)
	span.Attributes().PutStr("llm.request.model", "gpt-4o")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "llm.usage.prompt_tokens", 200)
	assertAttrStr(t, out, "llm.request.model", "gpt-4o")
	if _, ok := out.Get("gen_ai.usage.input_tokens"); ok {
		t.Error("OpenLLMetry attr should not be mapped when only openinference profile is enabled")
	}
}

func TestNoOpOnNonGenAISpan(t *testing.T) {
	cfg := &Config{Profiles: []string{"openinference", "openllmetry"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr("http.method", "GET")
	span.Attributes().PutInt("http.status_code", 200)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	if out.Len() != 2 {
		t.Errorf("expected 2 attributes unchanged, got %d", out.Len())
	}
}

func TestKeepOriginals(t *testing.T) {
	cfg := &Config{Profiles: []string{"openinference"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutInt("llm.token_count.prompt", 100)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "gen_ai.usage.input_tokens", 100)
	if _, ok := out.Get("llm.token_count.prompt"); !ok {
		t.Error("expected llm.token_count.prompt to be kept")
	}
}

func TestNormalizeLlamaIndex(t *testing.T) {
	cfg := &Config{Profiles: []string{"llamaindex"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	// Core token usage
	span.Attributes().PutInt("llm.token_count.prompt", 100)
	span.Attributes().PutInt("llm.token_count.completion", 200)
	span.Attributes().PutInt("llm.token_count.total", 300)
	// Token details
	span.Attributes().PutInt("llm.token_count.prompt_details.cache_read", 50)
	span.Attributes().PutInt("llm.token_count.completion_details.reasoning", 75)
	// Model info
	span.Attributes().PutStr("llm.model_name", "gpt-4o")
	span.Attributes().PutStr("llm.response.id", "chatcmpl-123")
	// LlamaIndex-specific
	span.Attributes().PutStr("query", "What is the capital of France?")
	span.Attributes().PutStr("answer", "Paris")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "gen_ai.usage.input_tokens", 100)
	assertAttrInt(t, out, "gen_ai.usage.output_tokens", 200)
	assertAttrInt(t, out, "gen_ai.usage.total_tokens", 300)
	assertAttrInt(t, out, "gen_ai.usage.prompt_details.cached_tokens", 50)
	assertAttrInt(t, out, "gen_ai.usage.completion_details.reasoning_tokens", 75)
	assertAttrStr(t, out, "gen_ai.request.model", "gpt-4o")
	assertAttrStr(t, out, "gen_ai.response.id", "chatcmpl-123")
	assertAttrStr(t, out, "gen_ai.prompt", "What is the capital of France?")
	assertAttrStr(t, out, "gen_ai.completion", "Paris")

	// Verify originals are removed
	if _, ok := out.Get("llm.token_count.prompt"); ok {
		t.Error("expected llm.token_count.prompt to be removed")
	}
	if _, ok := out.Get("query"); ok {
		t.Error("expected query to be removed")
	}
	if _, ok := out.Get("answer"); ok {
		t.Error("expected answer to be removed")
	}
}

func TestDynamicNestedToolDefinitions(t *testing.T) {
	cfg := &Config{Profiles: []string{"llamaindex"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()

	// Nested tool definitions with arbitrary depth (like llm.tools.N.tool.json_schema)
	span.Attributes().PutStr("llm.tools.0.tool.json_schema", `{"type":"object","properties":{"location":{"type":"string"}}}`)
	span.Attributes().PutStr("llm.tools.1.tool.json_schema", `{"type":"object","properties":{"query":{"type":"string"}}}`)
	span.Attributes().PutStr("llm.model_name", "gpt-4o")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	// Check that tool definitions were collected into a JSON array
	toolDefs, ok := out.Get("gen_ai.tool.definitions")
	if !ok {
		t.Fatal("expected gen_ai.tool.definitions to be set")
	}

	// Verify it's a valid JSON array
	var tools []map[string]interface{}
	if err := json.Unmarshal([]byte(toolDefs.Str()), &tools); err != nil {
		t.Fatalf("gen_ai.tool.definitions should be valid JSON: %v", err)
	}

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Verify originals are removed
	if _, ok := out.Get("llm.tools.0.tool.json_schema"); ok {
		t.Error("expected nested tool attributes to be removed")
	}
}

func TestLlamaIndexGenAISystemPropagation(t *testing.T) {
	cfg := &Config{Profiles: []string{"llamaindex"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	scopeSpans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

	// Span 1: Workflow span (no LLM provider info)
	span1 := scopeSpans.Spans().AppendEmpty()
	span1.SetName("AgentWorkflow.run_agent_step")
	span1.Attributes().PutStr("openinference.span.kind", "workflow")

	// Span 2: LLM call span (has llm.provider)
	span2 := scopeSpans.Spans().AppendEmpty()
	span2.SetName("OpenAI.astream_chat")
	span2.Attributes().PutStr("llm.provider", "openai")
	span2.Attributes().PutStr("llm.model_name", "gpt-4o")
	span2.Attributes().PutInt("llm.token_count.prompt", 100)

	// Span 3: Tool execution span (no LLM provider info)
	span3 := scopeSpans.Spans().AppendEmpty()
	span3.SetName("FunctionTool.acall")
	span3.Attributes().PutStr("openinference.span.kind", "tool")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	result := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// Verify all spans now have gen_ai.system="openai"
	for i := 0; i < result.Len(); i++ {
		span := result.At(i)
		attrs := span.Attributes()
		sys, ok := attrs.Get("gen_ai.system")
		if !ok {
			t.Errorf("span %d (%s) missing gen_ai.system", i, span.Name())
			continue
		}
		if sys.Str() != "openai" {
			t.Errorf("span %d (%s) gen_ai.system = %s, want openai", i, span.Name(), sys.Str())
		}
	}

	// Verify span 2 also has gen_ai.request.model (mapped from llm.model_name)
	span2Attrs := result.At(1).Attributes()
	assertAttrStr(t, span2Attrs, "gen_ai.request.model", "gpt-4o")
	assertAttrInt(t, span2Attrs, "gen_ai.usage.input_tokens", 100)
}

func TestLlamaIndexAgentNameExtraction(t *testing.T) {
	cfg := &Config{Profiles: []string{"llamaindex"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	scopeSpans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

	// Span 1: Has current_agent_name in output.value JSON (note: uses dot, not underscore)
	span1 := scopeSpans.Spans().AppendEmpty()
	span1.SetName("AgentRunner.run_step")
	span1.Attributes().PutStr("output.value", `{"current_agent_name":"MathTutorFunctionAgent","is_done":false}`)

	// Span 2: Has current_agent_name in input.value JSON (note: uses dot, not underscore)
	span2 := scopeSpans.Spans().AppendEmpty()
	span2.SetName("AgentRunner.process")
	span2.Attributes().PutStr("input.value", `{"current_agent_name":"ResearchAgent","task":"analyze"}`)

	// Span 3: LLM span with llm.provider for propagation
	span3 := scopeSpans.Spans().AppendEmpty()
	span3.SetName("OpenAI.chat")
	span3.Attributes().PutStr("llm.provider", "openai")

	// Span 4: Has gen_ai.agent.name already set (should not be overridden)
	span4 := scopeSpans.Spans().AppendEmpty()
	span4.SetName("AgentRunner.existing")
	span4.Attributes().PutStr("gen_ai.agent.name", "ExistingAgent")
	span4.Attributes().PutStr("output.value", `{"current_agent_name":"ShouldNotOverride"}`)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	result := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// Verify span 1 extracted agent name from output.value
	span1Attrs := result.At(0).Attributes()
	assertAttrStr(t, span1Attrs, "gen_ai.agent.name", "MathTutorFunctionAgent")
	assertAttrStr(t, span1Attrs, "gen_ai.system", "openai") // Should be propagated

	// Verify span 2 extracted agent name from input.value
	span2Attrs := result.At(1).Attributes()
	assertAttrStr(t, span2Attrs, "gen_ai.agent.name", "ResearchAgent")
	assertAttrStr(t, span2Attrs, "gen_ai.system", "openai") // Should be propagated

	// Verify span 3 has gen_ai.system from llm.provider
	span3Attrs := result.At(2).Attributes()
	assertAttrStr(t, span3Attrs, "gen_ai.system", "openai")

	// Verify span 4 kept existing gen_ai.agent.name
	span4Attrs := result.At(3).Attributes()
	assertAttrStr(t, span4Attrs, "gen_ai.agent.name", "ExistingAgent") // Should NOT be "ShouldNotOverride"
}

// --- AutoGen tests ---

// helper: build a scope-span with the given InstrumentationScope name and return the span.
// (Used by both OpenAI-scope tests and AutoGen-runtime tests.)
func newAutoGenSpan(td ptrace.Traces, scopeName string) ptrace.Span {
	ss := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	ss.Scope().SetName(scopeName)
	return ss.Spans().AppendEmpty()
}

// --- AutoGen: OpenAI instrumentation scope tests ---

// TestAutoGenOpenAITokenMapping verifies that llm.usage.* attributes on LLM spans
// (opentelemetry.instrumentation.openai.v1 scope) are renamed by the autogen profile.
func TestAutoGenOpenAITokenMapping(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	span.Attributes().PutInt("llm.usage.prompt_tokens", 489)
	span.Attributes().PutInt("llm.usage.completion_tokens", 28)
	span.Attributes().PutInt("llm.usage.total_tokens", 517)
	span.Attributes().PutStr("llm.request.model", "gpt-3.5-turbo")
	span.Attributes().PutStr("llm.request.type", "chat")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrInt(t, out, "gen_ai.usage.input_tokens", 489)
	assertAttrInt(t, out, "gen_ai.usage.output_tokens", 28)
	assertAttrInt(t, out, "gen_ai.usage.total_tokens", 517)
	assertAttrStr(t, out, "gen_ai.request.model", "gpt-3.5-turbo")
	assertAttrStr(t, out, "gen_ai.operation.name", "chat")
}

// TestAutoGenExtractToolNamesFromPromptIndexed verifies extraction of tool names from
// gen_ai.prompt.N.tool_calls.M.name attributes (opentelemetry.instrumentation.openai.v1 scope).
// Matches LangSmith format: gen_ai.tool.name (first tool) + gen_ai.tool.definitions (JSON string array).
func TestAutoGenExtractToolNamesFromPromptIndexed(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	// Two distinct tools used across the prompt history; add_numbers appears 3× (deduped)
	span.Attributes().PutStr("gen_ai.prompt.2.tool_calls.0.name", "add_numbers")
	span.Attributes().PutStr("gen_ai.prompt.6.tool_calls.0.name", "add_numbers")
	span.Attributes().PutStr("gen_ai.prompt.10.tool_calls.0.name", "add_numbers")
	span.Attributes().PutStr("gen_ai.prompt.10.tool_calls.1.name", "subtract_numbers")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	// gen_ai.tool.name is always a plain string (first sorted) — consistent with LangSmith/LlamaIndex.
	assertAttrStr(t, out, "gen_ai.tool.name", "add_numbers")

	// gen_ai.tool.definitions holds JSON string array of all called tools — same as LangSmith.
	defsVal, ok := out.Get("gen_ai.tool.definitions")
	if !ok {
		t.Fatal("expected gen_ai.tool.definitions to be set")
	}
	var defs []string
	if err := json.Unmarshal([]byte(defsVal.Str()), &defs); err != nil {
		t.Fatalf("gen_ai.tool.definitions is not valid JSON: %v", err)
	}
	if len(defs) != 2 || defs[0] != "add_numbers" || defs[1] != "subtract_numbers" {
		t.Errorf("unexpected gen_ai.tool.definitions: %v", defs)
	}
}

// TestAutoGenExtractToolNamesFromCompletion verifies that tool call names in the LLM response
// (gen_ai.completion.N.tool_calls.M.name) are also extracted — this covers the first LLM call
// in a multi-turn conversation where the tool calls appear only in the completion, not the prompt.
func TestAutoGenExtractToolNamesFromCompletion(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	// Tool calls appear only in the completion (first LLM turn — no prior tool results in prompt)
	span.Attributes().PutStr("gen_ai.completion.0.tool_calls.0.name", "add_numbers")
	span.Attributes().PutStr("gen_ai.completion.0.tool_calls.1.name", "subtract_numbers")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	assertAttrStr(t, out, "gen_ai.tool.name", "add_numbers")

	defsVal, ok := out.Get("gen_ai.tool.definitions")
	if !ok {
		t.Fatal("expected gen_ai.tool.definitions to be set")
	}
	var defs []string
	if err := json.Unmarshal([]byte(defsVal.Str()), &defs); err != nil {
		t.Fatalf("gen_ai.tool.definitions is not valid JSON: %v", err)
	}
	if len(defs) != 2 || defs[0] != "add_numbers" || defs[1] != "subtract_numbers" {
		t.Errorf("unexpected gen_ai.tool.definitions: %v", defs)
	}
}

// TestAutoGenPropagateAgentName verifies that gen_ai.agent.name is copied from the autogen
// runtime scope to the co-emitted OpenAI instrumentation scope within the same ResourceSpans.

func TestAutoGenPropagateAgentName(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	// Both scopes must be in the same ResourceSpans to test cross-scope propagation.
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()

	// Autogen runtime scope: send span with agent name (instance suffix to be stripped)
	autogenSS := rs.ScopeSpans().AppendEmpty()
	autogenSS.Scope().SetName("autogen SingleThreadedAgentRuntime")
	autogenSpan := autogenSS.Spans().AppendEmpty()
	autogenSpan.SetName("autogen send math_tutor.(default)-A")
	autogenSpan.Attributes().PutStr("messaging.destination", "math_tutor.(default)-A")
	autogenSpan.Attributes().PutStr("messaging.operation", "publish")

	// OpenAI instrumentation scope: LLM span — no agent name yet
	openaiSS := rs.ScopeSpans().AppendEmpty()
	openaiSS.Scope().SetName("opentelemetry.instrumentation.openai.v1")
	openaiSpan := openaiSS.Spans().AppendEmpty()
	openaiSpan.SetName("openai.chat")
	openaiSpan.Attributes().PutStr("gen_ai.operation.name", "chat")
	openaiSpan.Attributes().PutInt("gen_ai.usage.input_tokens", 100)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	outRS := sink.AllTraces()[0].ResourceSpans().At(0)

	// Autogen span: agent name cleaned up
	autogenOut := outRS.ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrStr(t, autogenOut, "gen_ai.agent.name", "math_tutor")

	// OpenAI span: agent name propagated from autogen scope
	openaiOut := outRS.ScopeSpans().At(1).Spans().At(0).Attributes()
	assertAttrStr(t, openaiOut, "gen_ai.agent.name", "math_tutor")
}

// TestAutoGenBuildInputMessagesSimple verifies that gen_ai.input.messages is assembled from
// simple gen_ai.prompt.N.{role,content} attributes (user/assistant text turns).
func TestAutoGenBuildInputMessagesSimple(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "system")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "You are a math tutor.")
	span.Attributes().PutStr("gen_ai.prompt.1.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.1.content", "What is 2+2?")
	span.Attributes().PutStr("gen_ai.completion.0.role", "assistant")
	span.Attributes().PutStr("gen_ai.completion.0.content", "The answer is 4.")
	span.Attributes().PutStr("gen_ai.completion.0.finish_reason", "stop")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	// gen_ai.input.messages must be a JSON array with two messages
	msgsVal, ok := out.Get("gen_ai.input.messages")
	if !ok {
		t.Fatal("expected gen_ai.input.messages to be set")
	}
	var msgs []map[string]interface{}
	if err := json.Unmarshal([]byte(msgsVal.Str()), &msgs); err != nil {
		t.Fatalf("gen_ai.input.messages is not valid JSON: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0]["role"] != "system" || msgs[0]["content"] != "You are a math tutor." {
		t.Errorf("unexpected msgs[0]: %v", msgs[0])
	}
	if msgs[1]["role"] != "user" || msgs[1]["content"] != "What is 2+2?" {
		t.Errorf("unexpected msgs[1]: %v", msgs[1])
	}

	// gen_ai.completion must be the assistant's final text
	assertAttrStr(t, out, "gen_ai.completion", "The answer is 4.")
}

// TestAutoGenBuildInputMessagesWithToolCalls verifies that tool-call turns (assistant messages
// with embedded tool_calls and subsequent tool-result messages) are correctly encoded.
func TestAutoGenBuildInputMessagesWithToolCalls(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	// Turn 0: user message
	span.Attributes().PutStr("gen_ai.prompt.0.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "add 5 and 3")
	// Turn 1: assistant requested two tool calls (no content)
	span.Attributes().PutStr("gen_ai.prompt.1.role", "assistant")
	span.Attributes().PutStr("gen_ai.prompt.1.tool_calls.0.id", "call_abc")
	span.Attributes().PutStr("gen_ai.prompt.1.tool_calls.0.name", "add_numbers")
	span.Attributes().PutStr("gen_ai.prompt.1.tool_calls.0.arguments", `{"a":5,"b":3}`)
	// Turn 2: tool result
	span.Attributes().PutStr("gen_ai.prompt.2.role", "tool")
	span.Attributes().PutStr("gen_ai.prompt.2.content", "8")
	span.Attributes().PutStr("gen_ai.prompt.2.tool_call_id", "call_abc")
	// Final completion (stop)
	span.Attributes().PutStr("gen_ai.completion.0.role", "assistant")
	span.Attributes().PutStr("gen_ai.completion.0.content", "5 + 3 = 8.")
	span.Attributes().PutStr("gen_ai.completion.0.finish_reason", "stop")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	msgsVal, ok := out.Get("gen_ai.input.messages")
	if !ok {
		t.Fatal("expected gen_ai.input.messages to be set")
	}
	var msgs []map[string]interface{}
	if err := json.Unmarshal([]byte(msgsVal.Str()), &msgs); err != nil {
		t.Fatalf("gen_ai.input.messages is not valid JSON: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// msgs[0]: user
	if msgs[0]["role"] != "user" || msgs[0]["content"] != "add 5 and 3" {
		t.Errorf("unexpected msgs[0]: %v", msgs[0])
	}
	// msgs[1]: assistant with tool_calls array
	if msgs[1]["role"] != "assistant" {
		t.Errorf("expected role=assistant, got %v", msgs[1]["role"])
	}
	tcs, ok := msgs[1]["tool_calls"].([]interface{})
	if !ok || len(tcs) != 1 {
		t.Fatalf("expected 1 tool call in msgs[1], got %v", msgs[1]["tool_calls"])
	}
	tc := tcs[0].(map[string]interface{})
	if tc["name"] != "add_numbers" {
		t.Errorf("unexpected tool name: %v", tc["name"])
	}
	// tool_calls.arguments must be decoded as a nested object, not a raw string
	if _, ok := tc["arguments"].(map[string]interface{}); !ok {
		t.Errorf("arguments should be a decoded JSON object, got %T", tc["arguments"])
	}
	// msgs[2]: tool result
	if msgs[2]["role"] != "tool" || msgs[2]["content"] != "8" {
		t.Errorf("unexpected msgs[2]: %v", msgs[2])
	}
	if msgs[2]["tool_call_id"] != "call_abc" {
		t.Errorf("missing tool_call_id on tool result: %v", msgs[2])
	}

	// completion
	assertAttrStr(t, out, "gen_ai.completion", "5 + 3 = 8.")
}

// TestAutoGenNoCompletionOnToolCallTurn verifies that gen_ai.completion is NOT set when
// the LLM responded with tool calls (finish_reason=tool_calls), only when it gave a final answer.
func TestAutoGenNoCompletionOnToolCallTurn(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	span.Attributes().PutStr("gen_ai.prompt.0.role", "user")
	span.Attributes().PutStr("gen_ai.prompt.0.content", "add 5 and 3")
	// LLM decided to call a tool — no text content in completion
	span.Attributes().PutStr("gen_ai.completion.0.role", "assistant")
	span.Attributes().PutStr("gen_ai.completion.0.finish_reason", "tool_calls")
	span.Attributes().PutStr("gen_ai.completion.0.tool_calls.0.name", "add_numbers")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	if _, ok := out.Get("gen_ai.completion"); ok {
		t.Error("gen_ai.completion should NOT be set when finish_reason=tool_calls")
	}
	// But input messages should still be set
	if _, ok := out.Get("gen_ai.input.messages"); !ok {
		t.Error("expected gen_ai.input.messages to be set")
	}
}

// TestAutoGenExtractToolDefinitionsFromFunctions verifies that llm.request.functions.N.*
// indexed attributes are combined into a structured gen_ai.tool.schemas JSON array
// (full schemas go to gen_ai.tool.schemas; gen_ai.tool.definitions is reserved for called-tool names).
func TestAutoGenExtractToolDefinitionsFromFunctions(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "opentelemetry.instrumentation.openai.v1")
	span.SetName("openai.chat")
	span.Attributes().PutStr("llm.request.functions.0.name", "add_numbers")
	span.Attributes().PutStr("llm.request.functions.0.description", "Add two numbers together")
	span.Attributes().PutStr("llm.request.functions.0.parameters", `{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`)
	span.Attributes().PutStr("llm.request.functions.1.name", "subtract_numbers")
	span.Attributes().PutStr("llm.request.functions.1.description", "Subtract b from a")
	span.Attributes().PutStr("llm.request.functions.1.parameters", `{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	// Full schemas now written to gen_ai.tool.schemas (not gen_ai.tool.definitions)
	schemasVal, ok := out.Get("gen_ai.tool.schemas")
	if !ok {
		t.Fatal("expected gen_ai.tool.schemas to be set")
	}
	var defs []map[string]any
	if err := json.Unmarshal([]byte(schemasVal.Str()), &defs); err != nil {
		t.Fatalf("gen_ai.tool.schemas is not valid JSON: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 tool schemas, got %d", len(defs))
	}
	if defs[0]["name"] != "add_numbers" {
		t.Errorf("defs[0].name = %v, want add_numbers", defs[0]["name"])
	}
	if defs[1]["name"] != "subtract_numbers" {
		t.Errorf("defs[1].name = %v, want subtract_numbers", defs[1]["name"])
	}
	// Parameters must be decoded as a nested object, not a raw string
	if _, ok := defs[0]["parameters"].(map[string]any); !ok {
		t.Errorf("defs[0].parameters should be a decoded JSON object, got %T", defs[0]["parameters"])
	}
	// Originals must be removed
	out.Range(func(k string, _ pcommon.Value) bool {
		if strings.HasPrefix(k, "llm.request.functions.") {
			t.Errorf("expected %s to be removed", k)
		}
		return true
	})
}

// TestAutoGenOpenAIScopeGuard verifies that OpenAI-scope tool extraction does NOT run
// for spans from an unrelated InstrumentationScope.
func TestAutoGenOpenAIScopeGuard(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "my.custom.instrumentation")
	span.SetName("some.operation")
	span.Attributes().PutStr("gen_ai.prompt.0.tool_calls.0.name", "some_tool")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	if _, ok := out.Get("gen_ai.tool.name"); ok {
		t.Error("gen_ai.tool.name should NOT be set for non-openai scope")
	}
}

// --- AutoGen: runtime span tests ---

// TestAutoGenCreateSpan covers the "autogen create <agent>" span type:
// messaging.destination is renamed + instance suffix stripped → gen_ai.agent.name,
// messaging.operation "create" → gen_ai.operation.name "create_agent",
// messaging.message.type is renamed → gen_ai.message.type.
func TestAutoGenCreateSpan(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "autogen SingleThreadedAgentRuntime")
	span.SetName("autogen create math_tutor.(default)-A")
	span.Attributes().PutStr("messaging.destination", "math_tutor.(default)-A")
	span.Attributes().PutStr("messaging.operation", "create")
	span.Attributes().PutStr("messaging.message.type", "UserMessage")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrStr(t, out, "gen_ai.agent.name", "math_tutor")
	assertAttrStr(t, out, "gen_ai.operation.name", "create_agent")
	assertAttrStr(t, out, "gen_ai.message.type", "UserMessage")

	// Originals removed
	for _, orig := range []string{"messaging.destination", "messaging.operation", "messaging.message.type"} {
		if _, ok := out.Get(orig); ok {
			t.Errorf("expected %s to be removed", orig)
		}
	}
}

// TestAutoGenSendSpan covers the "autogen send <agent>" span type:
// messaging.operation "publish" → gen_ai.operation.name "send_message".
func TestAutoGenSendSpan(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "autogen SingleThreadedAgentRuntime")
	span.SetName("autogen send math_tutor.(default)-A")
	span.Attributes().PutStr("messaging.destination", "math_tutor.(default)-A")
	span.Attributes().PutStr("messaging.operation", "publish")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrStr(t, out, "gen_ai.agent.name", "math_tutor")
	assertAttrStr(t, out, "gen_ai.operation.name", "send_message")
}

// TestAutoGenAckSpan covers the "autogen ack" span type:
// empty messaging.destination → gen_ai.agent.name attribute should NOT be present,
// messaging.operation "receive" → gen_ai.operation.name "receive_message".
func TestAutoGenAckSpan(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	span := newAutoGenSpan(td, "autogen SingleThreadedAgentRuntime")
	span.SetName("autogen ack")
	span.Attributes().PutStr("messaging.destination", "") // empty on ack spans
	span.Attributes().PutStr("messaging.operation", "receive")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	assertAttrStr(t, out, "gen_ai.operation.name", "receive_message")
	// Empty destination must be removed rather than kept as an empty string
	if _, ok := out.Get("gen_ai.agent.name"); ok {
		t.Error("expected gen_ai.agent.name to be absent on ack span with empty destination")
	}
}

// TestAutoGenScopeGuard verifies that spans from a non-AutoGen InstrumentationScope
// are NOT affected by the agent-name cleanup, even when messaging.* attributes are present.
func TestAutoGenScopeGuard(t *testing.T) {
	cfg := &Config{Profiles: []string{"autogen"}, RemoveOriginals: false}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	// Use a non-AutoGen scope (e.g. a Kafka consumer)
	span := newAutoGenSpan(td, "kafka-consumer")
	span.SetName("process orders.created")
	span.Attributes().PutStr("messaging.destination", "orders.created.(internal)-1")
	span.Attributes().PutStr("messaging.operation", "create")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	out := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	// Key is still renamed (profile mapping applies to all spans in the pipeline)
	// but the value must NOT have been cleaned up by normalizeAutoGenAttributes
	if agentName, ok := out.Get("gen_ai.agent.name"); ok {
		if agentName.Str() != "orders.created.(internal)-1" {
			t.Errorf("gen_ai.agent.name was unexpectedly modified for non-autogen scope: got %q", agentName.Str())
		}
	}
}

func TestLlamaIndexOutputValueExtraction(t *testing.T) {
	cfg := &Config{Profiles: []string{"llamaindex"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	scopeSpans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

	// Span 1: full raw with id, model, and finish_reason="tool_calls" (tool-calling turn)
	span1 := scopeSpans.Spans().AppendEmpty()
	span1.Attributes().PutStr("output.value", `{"response":{"role":"assistant"},"tool_calls":[{"tool_id":"call_abc","tool_name":"multiply"}],"raw":{"id":"chatcmpl-ABC123","model":"gpt-4o-2024-08-06","choices":[{"finish_reason":"tool_calls","index":0}],"usage":null},"current_agent_name":"calculator-agent"}`)

	// Span 2: raw with finish_reason="stop" (final answer turn)
	span2 := scopeSpans.Spans().AppendEmpty()
	span2.Attributes().PutStr("output.value", `{"response":{"role":"assistant","blocks":[{"text":"The result is 6."}]},"tool_calls":[],"raw":{"id":"chatcmpl-DEF456","model":"gpt-4o-2024-08-06","choices":[{"finish_reason":"stop","index":0}],"usage":{"completion_tokens":10,"prompt_tokens":100,"total_tokens":110}},"current_agent_name":"calculator-agent"}`)

	// Span 3: choices:[] — the streaming usage chunk (no finish_reason should be set)
	span3 := scopeSpans.Spans().AppendEmpty()
	span3.Attributes().PutStr("output.value", `{"response":{"role":"assistant","blocks":[{"text":"Done."}]},"tool_calls":[],"raw":{"id":"chatcmpl-GHI789","model":"gpt-3.5-turbo-0125","choices":[],"usage":{"completion_tokens":5,"prompt_tokens":50,"total_tokens":55}},"current_agent_name":"calculator-agent"}`)

	// Span 4: gen_ai.response.id and gen_ai.response.model already set — should NOT be overwritten;
	// finish_reason should still be extracted (always overwrite)
	span4 := scopeSpans.Spans().AppendEmpty()
	span4.Attributes().PutStr("gen_ai.response.id", "already-set-id")
	span4.Attributes().PutStr("gen_ai.response.model", "already-set-model")
	span4.Attributes().PutStr("output.value", `{"raw":{"id":"should-not-override","model":"should-not-override","choices":[{"finish_reason":"stop","index":0}]}}`)

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	result := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// Span 1: id, model, finish_reason all extracted
	span1Attrs := result.At(0).Attributes()
	assertAttrStr(t, span1Attrs, "gen_ai.response.id", "chatcmpl-ABC123")
	assertAttrStr(t, span1Attrs, "gen_ai.response.model", "gpt-4o-2024-08-06")
	assertAttrStr(t, span1Attrs, "gen_ai.response.finish_reasons", "tool_calls")

	// Span 2: id, model, finish_reason all extracted
	span2Attrs := result.At(1).Attributes()
	assertAttrStr(t, span2Attrs, "gen_ai.response.id", "chatcmpl-DEF456")
	assertAttrStr(t, span2Attrs, "gen_ai.response.model", "gpt-4o-2024-08-06")
	assertAttrStr(t, span2Attrs, "gen_ai.response.finish_reasons", "stop")

	// Span 3: choices:[] → id and model extracted, but finish_reason must NOT be set
	span3Attrs := result.At(2).Attributes()
	assertAttrStr(t, span3Attrs, "gen_ai.response.id", "chatcmpl-GHI789")
	assertAttrStr(t, span3Attrs, "gen_ai.response.model", "gpt-3.5-turbo-0125")
	if _, ok := span3Attrs.Get("gen_ai.response.finish_reasons"); ok {
		t.Error("expected gen_ai.response.finish_reasons to NOT be set when choices is empty")
	}

	// Span 4: id/model NOT overwritten (first-match-wins); finish_reason still extracted
	span4Attrs := result.At(3).Attributes()
	assertAttrStr(t, span4Attrs, "gen_ai.response.id", "already-set-id")
	assertAttrStr(t, span4Attrs, "gen_ai.response.model", "already-set-model")
	assertAttrStr(t, span4Attrs, "gen_ai.response.finish_reasons", "stop")
}

// TestOpenInferenceLangChainAgentName verifies that gen_ai.agent.name is set only on the
// root LangGraph span (user-defined name from compile(name=...)), and NOT on internal pipeline
// node spans that carry "langgraph_node" in their metadata JSON.
func TestOpenInferenceLangChainAgentName(t *testing.T) {
	cfg := &Config{Profiles: []string{"openinference"}, RemoveOriginals: true}
	sink := new(consumertest.TracesSink)
	p, err := createTracesProcessor(context.Background(), processortest.NewNopSettings(component.MustNewType(typeStr)), cfg, sink)
	if err != nil {
		t.Fatal(err)
	}

	td := ptrace.NewTraces()
	scopeSpans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()

	// Span 1: root graph span — no langgraph_node in metadata → should get gen_ai.agent.name from span name
	span1 := scopeSpans.Spans().AppendEmpty()
	span1.SetName("math_tutor_agent")
	span1.Attributes().PutStr("openinference.span.kind", "CHAIN")

	// Span 2: internal "agent" node — langgraph_node present → should NOT get gen_ai.agent.name
	span2 := scopeSpans.Spans().AppendEmpty()
	span2.SetName("agent")
	span2.Attributes().PutStr("openinference.span.kind", "CHAIN")
	span2.Attributes().PutStr("metadata", `{"langgraph_node":"agent","langgraph_step":1}`)

	// Span 3: internal "tools" node — langgraph_node present → should NOT get gen_ai.agent.name
	span3 := scopeSpans.Spans().AppendEmpty()
	span3.SetName("tools")
	span3.Attributes().PutStr("openinference.span.kind", "CHAIN")
	span3.Attributes().PutStr("metadata", `{"langgraph_node":"tools","langgraph_step":2}`)

	// Span 4: already has gen_ai.agent.name → must not be overridden
	span4 := scopeSpans.Spans().AppendEmpty()
	span4.SetName("some_graph")
	span4.Attributes().PutStr("openinference.span.kind", "CHAIN")
	span4.Attributes().PutStr("gen_ai.agent.name", "pre_existing_agent")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatal(err)
	}

	result := sink.AllTraces()[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// Span 1: root graph span gets agent name from span name
	span1Attrs := result.At(0).Attributes()
	assertAttrStr(t, span1Attrs, "gen_ai.agent.name", "math_tutor_agent")
	assertAttrStr(t, span1Attrs, "gen_ai.operation.name", "invoke_agent")

	// Span 2: internal "agent" node — no gen_ai.agent.name
	span2Attrs := result.At(1).Attributes()
	if _, ok := span2Attrs.Get("gen_ai.agent.name"); ok {
		t.Error("span 2 (langgraph_node=agent): gen_ai.agent.name should NOT be set on internal node spans")
	}

	// Span 3: internal "tools" node — no gen_ai.agent.name
	span3Attrs := result.At(2).Attributes()
	if _, ok := span3Attrs.Get("gen_ai.agent.name"); ok {
		t.Error("span 3 (langgraph_node=tools): gen_ai.agent.name should NOT be set on internal node spans")
	}

	// Span 4: pre-existing agent name preserved
	span4Attrs := result.At(3).Attributes()
	assertAttrStr(t, span4Attrs, "gen_ai.agent.name", "pre_existing_agent")
}

func assertAttrInt(t *testing.T, attrs pcommon.Map, key string, expected int64) {
	t.Helper()
	v, ok := attrs.Get(key)
	if !ok {
		t.Errorf("missing attribute %s", key)
		return
	}
	if v.Int() != expected {
		t.Errorf("%s = %d, want %d", key, v.Int(), expected)
	}
}

func assertAttrStr(t *testing.T, attrs pcommon.Map, key string, expected string) {
	t.Helper()
	v, ok := attrs.Get(key)
	if !ok {
		t.Errorf("missing attribute %s", key)
		return
	}
	if v.Str() != expected {
		t.Errorf("%s = %s, want %s", key, v.Str(), expected)
	}
}
