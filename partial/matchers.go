package partial

import (
	"protoc-gen-scip/scip"
	"strings"
)

func ToCamel(name string) string {
	boundName := ""
	for id, word := range strings.Split(strings.ToLower(name), "_") {
		if id == 0 {
			boundName += word
		} else {
			boundName += strings.Title(word)
		}
	}
	return name
}

type cppMatcher struct {
}

func (m *cppMatcher) GetLanguageStr() scip.Language {
	return scip.Language_CPP
}

func (m *cppMatcher) MatchServiceClass(defName string, protoName string) bool {
	return defName == "Service"
}

func (m *cppMatcher) MatchMethod(defName string, protoName string) bool {
	return defName == protoName
}

type javaMatcher struct {
}

func (m *javaMatcher) GetLanguageStr() scip.Language {
	return scip.Language_Java
}

func (m *javaMatcher) MatchServiceClass(defName string, protoName string) bool {
	return defName == protoName+"ImplBase"
}

func (m *javaMatcher) MatchMethod(defName string, protoName string) bool {
	return defName == ToCamel(protoName)
}
