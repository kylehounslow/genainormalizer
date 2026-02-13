package genainormalizer

// Config holds the configuration for the genainormalizer processor.
type Config struct {
	// Profiles to enable. Each profile maps a specific instrumentation
	// library's attributes to OTel GenAI Semantic Conventions.
	// Supported: openinference, openllmetry, langchain, crewai, pydanticai, strands
	Profiles []string `mapstructure:"profiles"`

	// RemoveOriginals deletes source attributes after mapping.
	RemoveOriginals bool `mapstructure:"remove_originals"`
}
