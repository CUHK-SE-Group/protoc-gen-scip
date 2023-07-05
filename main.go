package main

import (
	"flag"
	"fmt"
	"protoc-gen-scip/partial"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

const version = "0.0.1"

var scipFilePath *string
var outputFile *string

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")

	flag.Parse()
	if *showVersion {
		fmt.Printf("protoc-gen-lsif %v\n", version)
		return
	}

	var flags flag.FlagSet
	scipFilePath = flags.String("scip_file", "", "specify the path to the generated scip index")
	outputFile = flags.String("out_file", "out.scip", "specify the file to the newly updated scip")

	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			partial.GenerateFile(gen, f, *scipFilePath, *outputFile)
		}
		return nil
	})
}
