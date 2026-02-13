package genainormalizer

import (
	"context"
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
