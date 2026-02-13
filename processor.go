package genainormalizer

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
)

const typeStr = "genainormalizer"

type genaiNormalizerProcessor struct {
	next        consumer.Traces
	lookupTable map[string]string
	removeOrig  bool
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
		next:        next,
		lookupTable: BuildLookupTable(c.Profiles),
		removeOrig:  c.RemoveOriginals,
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
}

func (p *genaiNormalizerProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (p *genaiNormalizerProcessor) Start(_ context.Context, _ component.Host) error { return nil }
func (p *genaiNormalizerProcessor) Shutdown(_ context.Context) error                 { return nil }
