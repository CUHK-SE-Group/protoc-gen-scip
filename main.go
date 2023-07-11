package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"protoc-gen-scip/partial"

	"github.com/golang/glog"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

const version = "0.0.1"

var scipFilePath *string
var outputFile *string
var sourceroot *string

func init() {
	flag.Set("logtostderr", "false")
	flag.Set("stderrthreshold", "ERROR")
}

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")

	flag.Parse()
	if *showVersion {
		fmt.Printf("protoc-gen-lsif %v\n", version)
		return
	}

	var flags flag.FlagSet
	scipFilePath = flags.String("scip_dir", "", "specify the directory that contains the generated scip indexes")
	outputFile = flags.String("out_file", "out.scip", "specify the file to the newly updated scip")
	sourceroot = flags.String("sourceroot", "", "specify the ABSOLUTE source root in the unified output index")

	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		inputFiles := []*protogen.File{}
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			inputFiles = append(inputFiles, f)
		}

		scipFiles, err := filepath.Glob(filepath.Join(*scipFilePath, "*.scip"))

		if err != nil {
			glog.Fatalf("failed to scan the directory for scip index: %v", err)
		}
		if len(scipFiles) == 0 {
			glog.Errorf("no index to be analyzed")
			return nil
		}

		if !filepath.IsAbs(*sourceroot) {
			glog.Error("the source root is not an absolute path")
			*sourceroot = ""
		}
		partial.GenerateFile(gen, inputFiles, scipFiles, *outputFile, *sourceroot)
		return nil
	})
}
