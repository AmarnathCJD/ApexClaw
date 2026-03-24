package model

import "strings"

var baseModelMapping = map[string]string{
	"glm-4.5":      "0727-360B-API",
	"glm-4.6":      "GLM-4-6-API-V1",
	"glm-4.7":      "glm-4.7",
	"glm-5":        "glm-5",
	"glm-4.5-v":    "glm-4.5v",
	"glm-4.6-v":    "glm-4.6v",
	"glm-4.5-air":  "0727-106B-API",
	"0808-360b-dr": "0808-360B-DR",
}

func ParseModelName(model string) (base string, thinking bool, search bool) {
	base = strings.ToLower(model)
	for {
		if strings.HasSuffix(base, "-thinking") {
			thinking = true
			base = strings.TrimSuffix(base, "-thinking")
		} else if strings.HasSuffix(base, "-search") {
			search = true
			base = strings.TrimSuffix(base, "-search")
		} else if before, ok := strings.CutSuffix(base, "-tools"); ok {
			base = before
		} else {
			break
		}
	}
	return
}

func IsThinkingModel(model string) bool {
	_, t, _ := ParseModelName(model)
	return t
}

func IsSearchModel(model string) bool {
	_, _, s := ParseModelName(model)
	return s
}

func GetTargetModel(model string) string {
	base, _, _ := ParseModelName(model)
	if target, ok := baseModelMapping[base]; ok {
		return target
	}
	return "glm-4.7"
}
