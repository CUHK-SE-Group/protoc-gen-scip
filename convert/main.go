package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/hhatto/gocloc"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/proto"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"protoc-gen-scip/scip"
)

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
func convertCommand() cli.Command {
	var convertFlags convertFlags
	convert := cli.Command{
		Name:  "convert",
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
	return []*cli.Command{&convert, &cloccmd}
}
func main() {
	app := scipApp()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
