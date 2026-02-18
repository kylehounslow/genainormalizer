package genainormalizer

import (
	"context"
	"regexp"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
)

const typeStr = "genainormalizer"

type genaiNormalizerProcessor struct {
	next                consumer.Traces
	lookupTable         map[string]string
	removeOrig          bool
	flattenedSubkeyPats []*regexp.Regexp
}

// Patterns for flattened sub-keys that conflict with parent string values.
// OpenInference emits both "llm.input_messages" (JSON string) and
// "llm.input_messages.0.message.content" (flattened), which causes
// backends like OpenSearch to reject the document due to type conflicts.
var defaultFlattenedSubkeyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^llm\.input_messages\.\d+`),
	regexp.MustCompile(`^llm\.output_messages\.\d+`),
	regexp.MustCompile(`^gen_ai\.prompt\.\d+`),
	regexp.MustCompile(`^gen_ai\.completion\.\d+`),
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
	_ context.Context,
	_ processor.Settings,
	cfg component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	c := cfg.(*Config)
	return &genaiNormalizerProcessor{
		next:                next,
		lookupTable:         BuildLookupTable(c.Profiles),
		removeOrig:          c.RemoveOriginals,
		flattenedSubkeyPats: defaultFlattenedSubkeyPatterns,
	}, nil
}

func (p *genaiNormalizerProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		ilss := rss.At(i).ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			spans := ilss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				p.normalizeAttributes(spans.At(k).Attributes())
			}
		}
	}
	return p.next.ConsumeTraces(ctx, td)
}

func (p *genaiNormalizerProcessor) normalizeAttributes(attrs pcommon.Map) {
	var renames []struct{ from, to string }

	attrs.Range(func(k string, v pcommon.Value) bool {
		if target, ok := p.lookupTable[k]; ok {
			renames = append(renames, struct{ from, to string }{k, target})
		}
		return true
	})

	for _, r := range renames {
		if val, ok := attrs.Get(r.from); ok {
			// Apply value transformation if needed (e.g. span kind → operation name)
			if val.Type() == pcommon.ValueTypeStr {
				transformed := TransformValue(r.to, val.Str())
				if transformed != val.Str() {
					attrs.PutStr(r.to, transformed)
				} else {
					val.CopyTo(attrs.PutEmpty(r.to))
				}
			} else {
				val.CopyTo(attrs.PutEmpty(r.to))
			}
			if p.removeOrig {
				attrs.Remove(r.from)
			}
		}
	}

	// Remove flattened sub-keys that conflict with parent string values.
	p.stripFlattenedSubkeys(attrs)
}

func (p *genaiNormalizerProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (p *genaiNormalizerProcessor) stripFlattenedSubkeys(attrs pcommon.Map) {
	var toRemove []string
	attrs.Range(func(k string, _ pcommon.Value) bool {
		for _, pat := range p.flattenedSubkeyPats {
			if pat.MatchString(k) {
				toRemove = append(toRemove, k)
				break
			}
		}
		return true
	})
	for _, k := range toRemove {
		attrs.Remove(k)
	}
}

func (p *genaiNormalizerProcessor) Start(_ context.Context, _ component.Host) error { return nil }
func (p *genaiNormalizerProcessor) Shutdown(_ context.Context) error                 { return nil }
