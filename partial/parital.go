package partial

import (
	"io"
	"os"
	"protoc-gen-scip/scip"
	"strings"

	"github.com/golang/glog"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type symbolStringMap map[string]string

var newIndex *scip.Index
var typeMap map[string]*scipType
var globalSymbols symbolStringMap

type scipType struct {
	Name          string
	TypeSymbol    *scip.SymbolInformation
	Methods       []string
	MethodSymbols []*scip.SymbolInformation
}

func newScipType(name string, typeSymbol *scip.SymbolInformation, methods []string, methodSymbols []*scip.SymbolInformation) *scipType {
	return &scipType{
		Name:          name,
		TypeSymbol:    typeSymbol,
		Methods:       methods,
		MethodSymbols: methodSymbols,
	}
}

func (t *scipType) findMethod(s string) *scip.SymbolInformation {
	for idx, m := range t.Methods {
		if matchMethodName(m, s) {
			return t.MethodSymbols[idx]
		}
	}
	return nil
}

func getServiceKey(s *protogen.Service) string {
	return s.GoName
}

func getMethodKey(m *protogen.Method) string {
	return m.Parent.GoName + m.GoName
}

func registerServiceSymbolString(s *protogen.Service, symbol string) {
	globalSymbols[getServiceKey(s)] = symbol
}

func getSymbolStringForService(s *protogen.Service) string {
	return globalSymbols[getServiceKey(s)]
}

func registerMethodSymbolString(m *protogen.Method, symbol string) {
	globalSymbols[getMethodKey(m)] = symbol
}

func getMethodStringForService(m *protogen.Method) string {
	return globalSymbols[getMethodKey(m)]
}

func getKeyName(s string) string {
	return strings.ToLower(s)
}

func matchMethodName(s string, frag string) bool {
	return strings.Contains(strings.ReplaceAll(getKeyName(s), "_", ""), strings.ReplaceAll(getKeyName(frag), "_", ""))
}

func matchName(s string, frag string) bool {
	return strings.Contains(getKeyName(s), getKeyName(frag))
}

func matchProtoService(s *protogen.Service, t *scipType, symbols symbolStringMap) bool {
	if t.TypeSymbol == nil {
		glog.Fatalf("ill formed scip type: %v", *t)
	}

	if !matchName(t.Name, s.GoName) {
		return false
	}

	siMap := map[string]*scip.SymbolInformation{}
	for _, m := range s.Methods {
		if si := t.findMethod(m.GoName); si != nil {
			siMap[getMethodKey(m)] = si
		} else {
			return false
		}
	}

	t.TypeSymbol.Relationships = append(t.TypeSymbol.Relationships, &scip.Relationship{
		Symbol:           symbols[getServiceKey(s)],
		IsImplementation: true,
		IsReference:      true,
	})

	for key, si := range siMap {
		symbolString := symbols[key]
		si.Relationships = append(si.Relationships, &scip.Relationship{
			Symbol:           symbolString,
			IsReference:      true,
			IsImplementation: true,
		})
	}

	return true
}

func addScipTypeFromSymbolInformation(i *scip.SymbolInformation) {
	typeName := ""
	methodName := ""
	sym, err := scip.ParseSymbol(i.Symbol)
	if err != nil {
		glog.Errorf("can not parse the symbol %v", i)
		return
	}

	for _, desc := range sym.Descriptors {
		if desc.Suffix == scip.Descriptor_Type {
			typeName = typeName + "::" + desc.Name
		} else if desc.Suffix == scip.Descriptor_Method {
			methodName = desc.Name
		}
	}

	if typeName != "" && methodName != "" {
		if t, ok := typeMap[typeName]; ok {
			t.Methods = append(t.Methods, methodName)
			t.MethodSymbols = append(t.MethodSymbols, i)
		} else {
			typeMap[typeName] = newScipType(typeName, nil, []string{methodName}, []*scip.SymbolInformation{i})
		}
	} else if typeName != "" && methodName == "" {
		if t, ok := typeMap[typeName]; ok {
			t.TypeSymbol = i
		} else {
			typeMap[typeName] = newScipType(typeName, i, []string{}, []*scip.SymbolInformation{})
		}
	}
}

func visitMetadata(m *scip.Metadata) {
	newIndex.Metadata = m
}

func visitDocument(d *scip.Document) {
	newIndex.Documents = append(newIndex.Documents, d)
	for _, i := range d.Symbols {
		addScipTypeFromSymbolInformation(i)
	}
}

func visitExternalSymbol(e *scip.SymbolInformation) {
	newIndex.ExternalSymbols = append(newIndex.ExternalSymbols, e)
}

func makeOccurence(pos protoreflect.SourceLocation, symbol string) *scip.Occurrence {
	return &scip.Occurrence{
		Range:  []int32{int32(pos.StartLine), int32(pos.StartColumn), int32(pos.EndLine), int32(pos.EndColumn)},
		Symbol: symbol,
	}
}

func generateMethod(f *protogen.File, m *protogen.Method, d *scip.Document) {
	symbol := makeMethodSymbol(f, m)
	registerMethodSymbolString(m, symbol)

	symbolInfo := makeSymbolInformation(symbol, scip.SymbolInformation_UnspecifiedKind)
	occurence := makeOccurence(f.Desc.SourceLocations().ByPath(m.Location.Path), symbol)

	d.Symbols = append(d.Symbols, symbolInfo)
	d.Occurrences = append(d.Occurrences, occurence)
}

func generateService(f *protogen.File, s *protogen.Service, d *scip.Document) {
	symbol := makeServiceSymbol(f, s)
	registerServiceSymbolString(s, symbol)

	symbolInfo := makeSymbolInformation(symbol, scip.SymbolInformation_UnspecifiedKind)
	occurence := makeOccurence(f.Desc.SourceLocations().ByPath(s.Location.Path), symbol)

	d.Symbols = append(d.Symbols, symbolInfo)
	d.Occurrences = append(d.Occurrences, occurence)

	for _, m := range s.Methods {
		generateMethod(f, m, d)
	}
}

func generateProtoDocument(f *protogen.File) *scip.Document {
	protoDoc := &scip.Document{}
	protoDoc.RelativePath = *f.Proto.Name
	for _, s := range f.Services {
		generateService(f, s, protoDoc)
		matchCount := 0
		for _, t := range typeMap {
			if siMap := matchProtoService(s, t, globalSymbols); siMap {
				matchCount++
				glog.Infof("service %s matches: %v", s.GoName, siMap)
			}
		}
		if matchCount == 0 {
			glog.Fatalf("proto service implementation not found for %s", s.GoName)
		}
	}

	return protoDoc
}

func makeSymbolInformation(symbol string, symbolKind scip.SymbolInformation_Kind) *scip.SymbolInformation {
	return &scip.SymbolInformation{
		Symbol: symbol,
		Kind:   symbolKind,
	}
}

func makeMethodSymbol(f *protogen.File, method *protogen.Method) string {
	descriptors := []*scip.Descriptor{}
	for _, namespace := range strings.Split(f.GeneratedFilenamePrefix, "/") {
		descriptors = append(descriptors, &scip.Descriptor{Name: namespace, Suffix: scip.Descriptor_Namespace})
	}
	descriptors = append(descriptors, &scip.Descriptor{Name: method.Parent.GoName, Suffix: scip.Descriptor_Type})
	descriptors = append(descriptors, &scip.Descriptor{Name: method.GoName, Suffix: scip.Descriptor_Term})
	return scip.VerboseSymbolFormatter.FormatSymbol(&scip.Symbol{
		Scheme: "scip-proto",
		Package: &scip.Package{
			Manager: "proto",
			Name:    *f.Proto.Package,
			Version: *f.Proto.Syntax,
		},
		Descriptors: descriptors,
	})
}

func makeServiceSymbol(f *protogen.File, service *protogen.Service) string {
	descriptors := []*scip.Descriptor{}
	for _, namespace := range strings.Split(f.GeneratedFilenamePrefix, "/") {
		descriptors = append(descriptors, &scip.Descriptor{Name: namespace, Suffix: scip.Descriptor_Namespace})
	}
	descriptors = append(descriptors, &scip.Descriptor{Name: service.GoName, Suffix: scip.Descriptor_Type})
	return scip.VerboseSymbolFormatter.FormatSymbol(&scip.Symbol{
		Scheme: "scip-proto",
		Package: &scip.Package{
			Manager: "proto",
			Name:    *f.Proto.Package,
			Version: *f.Proto.Syntax,
		},
		Descriptors: descriptors,
	})
}

func GenerateFile(gen *protogen.Plugin, f *protogen.File, scipFilePath string, outputPath string) {
	newIndex = &scip.Index{}
	typeMap = map[string]*scipType{}
	globalSymbols = symbolStringMap{}

	visitor := scip.IndexVisitor{
		VisitMetadata:       visitMetadata,
		VisitDocument:       visitDocument,
		VisitExternalSymbol: visitExternalSymbol,
	}
	scipFile, err := os.Open(scipFilePath)
	if err != nil {
		glog.Fatalf("Error opening file: %s\n", err.Error())
	}
	defer scipFile.Close()
	is := io.Reader(scipFile)

	err = visitor.ParseStreaming(is)
	if err != nil {
		glog.Fatalf("error in visiting the scip file: %v", err)
	}

	protoDoc := generateProtoDocument(f)
	newIndex.Documents = append([]*scip.Document{scip.CanonicalizeDocument(protoDoc)}, newIndex.Documents...)
	// newIndex.Documents = []*scip.Document{scip.CanonicalizeDocument(protoDoc)}

	bytes, err := proto.Marshal(newIndex)
	if err != nil {
		glog.Fatalf("failed to generate protobuf of the newly updated index: %v", err)
	}

	g := gen.NewGeneratedFile(outputPath, f.GoImportPath)
	g.Write(bytes)
}
