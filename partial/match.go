package partial

import "protoc-gen-scip/scip"

type Matcher interface {
	GetLanguageStr() scip.Language
	MatchServiceClass(defName string, protoName string) bool
	MatchMethod(defname string, protoName string) bool
}

type MatcherSelector struct {
	matcherMap map[string][]Matcher
}

// func NewDefaultMatcherSelector() *MatcherSelector {
// 	return &MatcherSelector{
// 		matcherMap: ,
// 	}
// }
