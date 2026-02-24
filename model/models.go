package model

import "strings"

var baseModelMapping = map[string]string{
	"GLM-5":   "glm-5",
	"GLM-4.7": "glm-4.7",
}

func ParseModelName(model string) (base string, thinking bool, search bool) {
	base = model
	for {
		if strings.HasSuffix(base, "-thinking") {
			thinking = true
			base = strings.TrimSuffix(base, "-thinking")
		} else if strings.HasSuffix(base, "-search") {
			search = true
			base = strings.TrimSuffix(base, "-search")
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
	return model
}
