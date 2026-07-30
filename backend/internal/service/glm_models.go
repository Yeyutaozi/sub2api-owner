package service

var defaultGLMModelIDs = []string{
	"glm-5.2",
	"glm-5.1",
	"glm-5",
	"glm-5-turbo",
	"glm-4.7",
	"glm-4.7-flashx",
	"glm-4.7-flash",
	"glm-4.6",
	"glm-4.5",
	"glm-4.5-x",
	"glm-4.5-air",
	"glm-4.5-airx",
	"glm-4.5-flash",
	"glm-4-32b-0414-128k",
	"embedding-3",
}

// DefaultGLMModelIDs returns the built-in GLM model catalog used when an
// upstream model list is not available.
func DefaultGLMModelIDs() []string {
	return append([]string(nil), defaultGLMModelIDs...)
}
