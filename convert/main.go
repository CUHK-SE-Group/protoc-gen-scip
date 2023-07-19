package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hhatto/gocloc"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"protoc-gen-scip/scip"
)

const referenceRelStr = "reference"
const implementationRelStr = "implementation"
const definitionRelStr = "definition"

type convertFlags struct {
	from    string
	to      string
	verbose bool
}

func readFromOption(fromPath string) (*scip.Index, error) {
	var scipReader io.Reader
	if fromPath == "-" {
		scipReader = os.Stdin
	} else if !strings.HasSuffix(fromPath, ".scip") && !strings.HasSuffix(fromPath, ".lsif-typed") {
		return nil, errors.Newf("expected file with .scip extension but found %s", fromPath)
	} else {
		scipFile, err := os.Open(fromPath)
		defer scipFile.Close()
		if err != nil {
			return nil, err
		}
		scipReader = scipFile
	}

	scipBytes, err := io.ReadAll(scipReader)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read SCIP index at path %s", fromPath)
	}

	scipIndex := scip.Index{}
	err = proto.Unmarshal(scipBytes, &scipIndex)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse SCIP index at path %s", fromPath)
	}
	return &scipIndex, nil
}

func getTypeName(str string) string {
	if split := strings.SplitAfter(str, "/"); len(split) > 1 {
		return split[len(split)-1]
	}
	return str
}

func genDocument(d *scip.Document, index *scip.Index) []byte {
	content := []byte(fmt.Sprintf("CREATE (:document{abspath:\"%s\", relpath:\"%s\", lang:\"%s\"});\n", filepath.Join(index.Metadata.ProjectRoot, d.RelativePath), d.RelativePath, d.Language))
	return content
}

func genSymbol(sym *scip.SymbolInformation, d *scip.Document) []byte {
	var create []byte
	if strings.HasSuffix(d.RelativePath, ".proto") {
		create = []byte(fmt.Sprintf("CREATE (:protosym{name:\"%s\", fullname:\"%s\", docrelpath:\"%s\"});\n", getTypeName(sym.Symbol), sym.Symbol, d.RelativePath))

	} else {
		create = []byte(fmt.Sprintf("CREATE (:symbol{name:\"%s\", fullname:\"%s\", docrelpath:\"%s\"});\n", getTypeName(sym.Symbol), sym.Symbol, d.RelativePath))

	}
	match := []byte(fmt.Sprintf("MATCH (n0 {relpath:\"%s\"}), (n1 {fullname:\"%s\", docrelpath:\"%s\"}) MERGE(n0)-[:contains]->(n1);\n", d.RelativePath, sym.Symbol, d.RelativePath))
	return append(create, match...)
}

func genRelationShip(rel *scip.Relationship, sym *scip.SymbolInformation, relationStr string) []byte {
	match := []byte(fmt.Sprintf("MATCH (n0 {fullname:\"%s\"}), (n1 {fullname:\"%s\"}) MERGE(n0)-[:%s]->(n1);\n", sym.Symbol, rel.GetSymbol(), relationStr))
	return match
}

func genRefers(sym string, d *scip.Document) []byte {
	contains := []byte(fmt.Sprintf("MATCH (n0 {relpath:\"%s\"}), (n1 {fullname:\"%s\"}) MERGE(n0)-[:refers]->(n1);\n", d.RelativePath, sym))
	return contains
}
func convertSCIPToMemgraph(index *scip.Index) []byte {
	content := []byte{}
	for _, d := range index.Documents {
		content = append(content, genDocument(d, index)...)
		for _, s := range d.Symbols {
			content = append(content, genSymbol(s, d)...)
		}
	}

	for _, d := range index.Documents {
		for _, s := range d.Symbols {
			for _, r := range s.Relationships {
				if r.IsDefinition {
					content = append(content, genRelationShip(r, s, definitionRelStr)...)
				}
				if r.IsImplementation {
					content = append(content, genRelationShip(r, s, implementationRelStr)...)
				}
				if r.IsReference {
					content = append(content, genRelationShip(r, s, referenceRelStr)...)
				}

			}
		}
		for _, o := range d.Occurrences {
			content = append(content, genRefers(o.Symbol, d)...)
		}
	}
	return content
}
func memgraphMain(flags convertFlags) error {
	scipIndex, err := readFromOption(flags.from)
	if err != nil {
		return err
	}

	var memgraphWriter io.Writer
	toPath := flags.to
	if toPath == "-" {
		memgraphWriter = os.Stdout
		// } else if !strings.HasSuffix(toPath, ".lsif") {
		// 	return errors.Newf("expected file with .lsif extension but found %s", toPath)
	} else {
		graphDefinitions, err := os.OpenFile(toPath, os.O_WRONLY|os.O_CREATE, 0666)
		defer graphDefinitions.Close()
		if err != nil {
			return err
		}
		memgraphWriter = graphDefinitions
	}

	lsifIndex := convertSCIPToMemgraph(scipIndex)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to convert SCIP index to LSIF index")
	// }

	_, err = memgraphWriter.Write(lsifIndex)
	if err != nil {
		return errors.Wrapf(err, "failed to write LSIF index to path %s", toPath)
	}

	return nil
}
func tomemgraph() cli.Command {
	var convertFlags convertFlags
	convert := cli.Command{
		Name:  "convert2cypher",
		Usage: "Convert a SCIP index to memgrph...",
		Flags: []cli.Flag{
			fromFlag(&convertFlags.from),
			&cli.StringFlag{
				Name:        "to",
				Usage:       "Output path for memgraph definitions",
				Destination: &convertFlags.to,
				Value:       "memgraph.out",
			},
		},
		Action: func(c *cli.Context) error {
			return memgraphMain(convertFlags)
		},
	}
	return convert
}
func convertCommand() cli.Command {
	var convertFlags convertFlags
	convert := cli.Command{
		Name:  "convert2lsif",
		Usage: "Convert a SCIP index to an LSIF index",
		Flags: []cli.Flag{
			fromFlag(&convertFlags.from),
			&cli.StringFlag{
				Name:        "to",
				Usage:       "Output path for LSIF index",
				Destination: &convertFlags.to,
				Value:       "dump.lsif",
			},
		},
		Action: func(c *cli.Context) error {
			return convertMain(convertFlags)
		},
	}
	return convert
}
func clocCommand() cli.Command {
	var convertFlags convertFlags
	convert := cli.Command{
		Name:  "cloc",
		Usage: "Count a SCIP index's Lines of Code",
		Flags: []cli.Flag{
			fromFlag(&convertFlags.from),
			&cli.BoolFlag{
				Name:        "verbose",
				Usage:       "output cloc files",
				Destination: &convertFlags.verbose,
				Value:       false,
			},
		},
		Action: func(c *cli.Context) error {
			return clocMain(convertFlags)
		},
	}
	return convert
}

func clocMain(flags convertFlags) error {
	scipIndex, err := readFromOption(flags.from)
	if err != nil {
		return err
	}
	root := scipIndex.Metadata.ProjectRoot
	var files []string
	for _, doc := range scipIndex.Documents {
		files = append(files, doc.RelativePath)
	}
	languages := gocloc.NewDefinedLanguages()
	options := gocloc.NewClocOptions()
	options.SkipDuplicated = true
	var paths []string
	root = strings.TrimPrefix(root, "file://")
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}
	for _, v := range files {
		paths = append(paths, path.Join(root, v))
	}
	if flags.verbose {
		for _, v := range paths {
			fmt.Println(v)
		}
	}
	processor := gocloc.NewProcessor(languages, options)
	result, err := processor.Analyze(paths)
	if err != nil {
		fmt.Printf("gocloc fail. error: %v\n", err)
		return err
	}

	for k, v := range result.Languages {
		fmt.Printf("Language: %v, Loc: %v\n", k, v.Code)
	}
	return nil
}
func convertMain(flags convertFlags) error {
	scipIndex, err := readFromOption(flags.from)
	if err != nil {
		return err
	}

	var lsifWriter io.Writer
	toPath := flags.to
	if toPath == "-" {
		lsifWriter = os.Stdout
	} else if !strings.HasSuffix(toPath, ".lsif") {
		return errors.Newf("expected file with .lsif extension but found %s", toPath)
	} else {
		lsifFile, err := os.OpenFile(toPath, os.O_WRONLY|os.O_CREATE, 0666)
		defer lsifFile.Close()
		if err != nil {
			return err
		}
		lsifWriter = lsifFile
	}

	lsifIndex, err := scip.ConvertSCIPToLSIF(scipIndex)
	if err != nil {
		return errors.Wrap(err, "failed to convert SCIP index to LSIF index")
	}

	err = scip.WriteNDJSON(scip.ElementsToJsonElements(lsifIndex), lsifWriter)
	if err != nil {
		return errors.Wrapf(err, "failed to write LSIF index to path %s", toPath)
	}

	return nil
}

func scipApp() *cli.App {
	app := &cli.App{
		Name:        "scip",
		Version:     "0.0.1",
		Usage:       "SCIP Code Intelligence Protocol CLI",
		Description: "For more details, see the project README at:\n\n\thttps://github.com/sourcegraph/scip",
		Commands:    commands(),
	}
	return app
}
func fromFlag(storage *string) *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "from",
		Usage:       "Path to SCIP index file",
		Destination: storage,
		Value:       "index.scip",
	}
}

func commands() []*cli.Command {
	convert := convertCommand()
	cloccmd := clocCommand()
	tomem := tomemgraph()
	return []*cli.Command{&convert, &cloccmd, &tomem}
}
func main() {
	app := scipApp()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
