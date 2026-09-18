package domain

// GroupModelsListConfig controls the optional custom /v1/models response list.
// It is retained for backward compatibility with existing application-center
// groups; the newer ModelAllowlist field provides stricter request admission.
type GroupModelsListConfig struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models,omitempty"`
}
