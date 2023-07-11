package partial

import (
	"io"
	"os"
	"path/filepath"
	"protoc-gen-scip/scip"
	"strings"
	"sync"

	"github.com/golang/glog"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// type symbolStringMap map[string]string

var indexes []*scip.Index
var typeMaps []map[string]*scipType
var whiteListedSymbols sync.Map

var grpcImpls sync.Map

// var globalSymbols symbolStringMap

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

func getKeyName(s string) string {
	return strings.ToLower(s)
}

func matchMethodName(s string, frag string) bool {
	return strings.Contains(strings.ReplaceAll(getKeyName(s), "_", ""), strings.ReplaceAll(getKeyName(frag), "_", ""))
}

func matchName(s string, frag string) bool {
	return strings.Contains(getKeyName(s), getKeyName(frag))
}

func matchProtoService(s *protogen.Service, t *scipType, symbols map[string]*scip.SymbolInformation, relations map[string][]*scip.Relationship) (map[string][]*scip.Relationship, bool) {
	if t.TypeSymbol == nil {
		glog.Infof("ill formed scip type: %v", *t)
		return relations, false
	}

	if !matchName(t.Name, s.GoName) {
		return relations, false
	}

	siMap := map[string]*scip.SymbolInformation{}
	siMap[getServiceKey(s)] = t.TypeSymbol
	for _, m := range s.Methods {
		if si := t.findMethod(m.GoName); si != nil {
			siMap[getMethodKey(m)] = si
		} else {
			return relations, false
		}
	}

	for key, si := range siMap {
		whiteListedSymbols.Store(si.Symbol, struct{}{})
		grpcImpls.Store(si.Symbol, symbols[key].Symbol)
		si.Relationships = append(si.Relationships, &scip.Relationship{
			Symbol:           symbols[key].Symbol,
			IsReference:      true,
			IsImplementation: true,
		})
		if _, ok := relations[key]; !ok {
			relations[key] = []*scip.Relationship{}
		}
		relations[key] = append(relations[key], &scip.Relationship{
			Symbol:      si.Symbol,
			IsReference: true,
		})
	}
	glog.Infof("service %s matches: %s", s.GoName, t.TypeSymbol.Symbol)

	return relations, true
}

func addScipTypeFromSymbolInformation(mapId int, i *scip.SymbolInformation) {
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
		} else if desc.Suffix == scip.Descriptor_Method || desc.Suffix == scip.Descriptor_Term {
			methodName = desc.Name
		}
	}

	if typeName != "" && methodName != "" {
		if t, ok := typeMaps[mapId][typeName]; ok {
			t.Methods = append(t.Methods, methodName)
			t.MethodSymbols = append(t.MethodSymbols, i)
		} else {
			typeMaps[mapId][typeName] = newScipType(typeName, nil, []string{methodName}, []*scip.SymbolInformation{i})
		}
	} else if typeName != "" && methodName == "" {
		if t, ok := typeMaps[mapId][typeName]; ok {
			t.TypeSymbol = i
		} else {
			typeMaps[mapId][typeName] = newScipType(typeName, i, []string{}, []*scip.SymbolInformation{})
		}
	}
}

func filter(d *scip.Document) bool {
	return true
}

func makeOccurence(pos protoreflect.SourceLocation, symbol string) *scip.Occurrence {
	return &scip.Occurrence{
		Range:  []int32{int32(pos.StartLine), int32(pos.StartColumn), int32(pos.EndLine), int32(pos.EndColumn)},
		Symbol: symbol,
	}
}

func generateMethod(f *protogen.File, m *protogen.Method, d *scip.Document) *scip.SymbolInformation {
	symbol := makeMethodSymbol(f, m)

	symbolInfo := makeSymbolInformation(symbol, scip.SymbolInformation_UnspecifiedKind)
	occurence := makeOccurence(f.Desc.SourceLocations().ByPath(m.Location.Path), symbol)

	d.Symbols = append(d.Symbols, symbolInfo)
	d.Occurrences = append(d.Occurrences, occurence)

	return symbolInfo
}

func generateService(f *protogen.File, s *protogen.Service, d *scip.Document) map[string]*scip.SymbolInformation {
	siMap := map[string]*scip.SymbolInformation{}
	symbol := makeServiceSymbol(f, s)

	symbolInfo := makeSymbolInformation(symbol, scip.SymbolInformation_UnspecifiedKind)
	occurence := makeOccurence(f.Desc.SourceLocations().ByPath(s.Location.Path), symbol)

	d.Symbols = append(d.Symbols, symbolInfo)
	d.Occurrences = append(d.Occurrences, occurence)
	siMap[getServiceKey(s)] = symbolInfo

	for _, m := range s.Methods {
		siMap[getMethodKey(m)] = generateMethod(f, m, d)
	}

	return siMap
}

func generateProtoDocument(f *protogen.File, sourceroot string) *scip.Document {
	protoDoc := &scip.Document{}
	absFilePath, err := filepath.Abs(*f.Proto.Name)
	if err != nil {
		glog.Errorf("can not get the absolute path of the input proto: %v", err)
		glog.Errorf("the filename is: %s", *f.Proto.Name)
		sourceroot = ""
		absFilePath = *f.Proto.Name
	}

	if sourceroot != "" {
		relPath, err := filepath.Rel(sourceroot, absFilePath)
		if err != nil {
			glog.Errorf("can not get the relative path for the new proto document: %v", err)
			glog.Errorf("the sourceroot is %s, and the absolute file path is %s", sourceroot, absFilePath)
			relPath = *f.Proto.Name
		}
		protoDoc.RelativePath = relPath
	} else {
		protoDoc.RelativePath = *f.Proto.Name
	}

	for _, s := range f.Services {
		siMap := generateService(f, s, protoDoc)
		numGoroutines := len(typeMaps)
		relationMapChan := make(chan map[string][]*scip.Relationship, numGoroutines)
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for _, types := range typeMaps {
			scipTypes := types
			matched := false
			go func() {
				relations := make(map[string][]*scip.Relationship)
				for _, t := range scipTypes {
					relations, matched = matchProtoService(s, t, siMap, relations)
				}
				if !matched {
					relationMapChan <- relations
				}
				wg.Done()
			}()
		}

		wg.Wait()

		close(relationMapChan)

		if len(relationMapChan) == 0 {
			glog.Errorf("proto service implementation not found for %s", s.GoName)
			glog.Errorf("skip the service: %s", s.GoName)
			continue
		}

		// for m := range relationMapChan {
		// 	// for key, rels := range m {
		// 	// 	// siMap[key].Relationships = append(siMap[key].Relationships, rels...)
		// 	// }
		// }

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

func removePrefix(path string) string {
	newPath := strings.TrimPrefix(path, "file://")
	if !strings.HasPrefix(newPath, "/") {
		return "/" + newPath
	}
	return newPath
}

func appendPrefix(path string) string {
	if !strings.HasPrefix(path, "file://") {
		return "file://" + path
	}
	return path
}

func indexScipFile(id int, scipFilePath string, sourceroot string, wg *sync.WaitGroup) {
	defer wg.Done()
	visitDocument := func(d *scip.Document) {
		absDocPath := filepath.Join(removePrefix(indexes[id].Metadata.GetProjectRoot()), d.RelativePath)
		absDocPath = filepath.Clean(absDocPath)
		newRelPath, err := filepath.Rel(sourceroot, absDocPath)
		if err != nil {
			glog.Errorf("can not get the new relative path for %s: %v", scipFilePath, err)
			newRelPath = d.RelativePath
		}
		d.RelativePath = newRelPath
		indexes[id].Documents = append(indexes[id].Documents, d)
		if filter(d) {
			for _, i := range d.Symbols {
				addScipTypeFromSymbolInformation(id, i)
			}
		}
	}

	visitMetadata := func(m *scip.Metadata) {
		indexes[id].Metadata = m
	}

	visitExternalSymbol := func(e *scip.SymbolInformation) {
		// indexes[id].ExternalSymbols = append(indexes[id].ExternalSymbols, e)
	}

	visitor := scip.IndexVisitor{
		VisitMetadata:       visitMetadata,
		VisitDocument:       visitDocument,
		VisitExternalSymbol: visitExternalSymbol,
	}

	scipFile, err := os.Open(scipFilePath)
	if err != nil {
		glog.Errorf("Error opening file: %s\n", err.Error())
		glog.Errorf("skip that file: %s", scipFilePath)
		indexes[id] = &scip.Index{}
		return
	}
	defer scipFile.Close()
	is := io.Reader(scipFile)

	err = visitor.ParseStreaming(is)
	if err != nil {
		glog.Errorf("error in visiting the scip file: %v", err)
		glog.Errorf("skip that file: %s", scipFilePath)
		indexes[id] = &scip.Index{}
		return
	}

	if indexes[id].Metadata == nil {
		glog.Errorf("Metada is nil in %s: maybe the index is empty? ", scipFilePath)
		indexes[id].Metadata = &scip.Metadata{}
	}

	indexes[id].Metadata.ProjectRoot = appendPrefix(sourceroot)
}

func findRelationSymbol(rels []*scip.Relationship, symbol string) bool {
	for _, rel := range rels {
		if rel.Symbol == symbol {
			return true
		}
	}
	return false
}

func appendProtoRef(s *scip.SymbolInformation) *scip.SymbolInformation {
	newRelations := []*scip.Relationship{}
	for _, rel := range s.Relationships {
		if protoSym, ok := grpcImpls.Load(rel.Symbol); ok {
			if !findRelationSymbol(s.Relationships, protoSym.(string)) {
				newRelations = append(newRelations, &scip.Relationship{
					Symbol:           protoSym.(string),
					IsReference:      true,
					IsImplementation: true,
				})
			}
		}
	}

	s.Relationships = append(s.Relationships, newRelations...)

	return s
}

func filterDocument(d *scip.Document) *scip.Document {
	ret := &scip.Document{}
	for _, sym := range d.Symbols {
		if _, ok := whiteListedSymbols.Load(sym.Symbol); ok {
			sym = appendProtoRef(sym)
			ret.Symbols = append(ret.Symbols, sym)
		} else {
			for _, rel := range sym.Relationships {
				if _, ok := whiteListedSymbols.Load(rel.Symbol); ok {
					whiteListedSymbols.Store(sym.Symbol, struct{}{})
					sym = appendProtoRef(sym)
					ret.Symbols = append(ret.Symbols, sym)
					break
				}
			}
		}
	}
	for _, o := range d.Occurrences {
		if _, ok := whiteListedSymbols.Load(o.Symbol); ok {
			ret.Occurrences = append(ret.Occurrences, o)
		}
	}

	ret.Language = d.Language
	ret.RelativePath = d.RelativePath
	ret.Text = d.Text
	return ret
}

func mergeIndexes(indexes []*scip.Index, newIndex *scip.Index) *scip.Index {
	if len(indexes) == 0 {
		glog.Infof("no index to be merged.")
		return newIndex
	}

	newIndex.Metadata = indexes[0].Metadata
	for _, i := range indexes {
		for _, d := range i.Documents {
			newDoc := filterDocument(d)
			if len(newDoc.Symbols) != 0 || len(newDoc.Occurrences) != 0 {
				newIndex.Documents = append(newIndex.Documents, newDoc)
			}
		}
		// newIndex.ExternalSymbols = append(newIndex.ExternalSymbols, i.ExternalSymbols...)
	}

	return newIndex
}

func GenerateFile(gen *protogen.Plugin, files []*protogen.File, scipFilePaths []string, outputPath string, sourceroot string) {
	indexes = make([]*scip.Index, len(scipFilePaths))
	whiteListedSymbols = sync.Map{}
	newIndex := &scip.Index{}
	for i := range indexes {
		indexes[i] = &scip.Index{}
	}
	typeMaps = make([]map[string]*scipType, len(scipFilePaths))
	for i := range typeMaps {
		typeMaps[i] = map[string]*scipType{}
	}
	// globalSymbols = symbolStringMap{}

	numGoroutines := len(scipFilePaths)
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for id, path := range scipFilePaths {
		go indexScipFile(id, path, sourceroot, &wg)
	}

	wg.Wait()
	protoDocs := []*scip.Document{}
	for _, f := range files {
		protoDoc := generateProtoDocument(f, sourceroot)
		protoDocs = append(protoDocs, protoDoc)
		// newIndex.Documents = append([]*scip.Document{scip.CanonicalizeDocument(protoDoc)}, newIndex.Documents...)
	}

	newIndex = mergeIndexes(indexes, newIndex)
	newIndex.Documents = append(protoDocs, newIndex.Documents...)

	bytes, err := proto.Marshal(newIndex)
	if err != nil {
		glog.Fatalf("failed to generate protobuf of the newly updated index: %v", err)
	}

	g := gen.NewGeneratedFile(outputPath, "")
	g.Write(bytes)
}
