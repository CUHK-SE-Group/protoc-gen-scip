# Introduction

In this repository, there are two binaries. 

- `protoc-gen-scip`, a protoc compiler plugin
- `tool`, a utility tool

`protoc-gen-scip` is used to merge the original SCIP file and the Protobuf definition files, i.e., if you have generated some projects' SCIP index, and there are some gRPC calls/relations between these projects, you can use this plugin to build this gRPC relationships among projects.

The workflow is shown below. 
- First, you should build each projects' SCIP index. 
- Then, you can use `protoc-gen-scip` to merge these SCIP index, and merge the gRPC Proto Definitions as well.
- At last, you can use `tool` to convert it into `LISF` format, or `cypherl` file for further analysis.

![workflow](docs/workflow.jpg)

We have a [benchmark](https://github.com/CUHK-SE-Group/RPCoverBenchmark) to test the functionality and performance of our plugin.
# Quick Start

## protoc-gen-scip

Run the following command, two binaries will be generated, namely `protoc-gen-scip` and `tool`

```
make all
```

## Example Usage

There are some parameters for `protoc-gen-scip`.

- `--scip_out`, the path that final scip file will be generated
- `--plugin`, specify the plugin we will use. In this case is `protoc-gen-scip`
- `--scip_opt`, the actual parameter which will be passed to the plugin
    - `scip_dir`, the input path of orginal scip files.
    - `sourceroot`, the root path of these scipfiles
    - `out_file`, the final generated file name.
- `-I`, specify the proto path
- `$(find . -name "*.proto")`, the proto files name. In this case, it will find all the proto files in the current directory

```shell
protoc --scip_out=./ --plugin=protoc-gen-scip --scip_opt=scip_dir=./,sourceroot=$(pwd),out_file=total.scip -I . $(find . -name "*.proto")
```

## tool

tool have three subcommand:

- count the lines of code in a SCIP index file. 
- convert the SCIP index file into LSIF.
- convert the SCIP index file into Cypher.

```bash
$ ./tool                                               
NAME:
   scip - SCIP Code Intelligence Protocol CLI

USAGE:
   scip [global options] command [command options] [arguments...]

VERSION:
   0.0.1

DESCRIPTION:
   For more details, see the project README at:

     https://github.com/sourcegraph/scip

COMMANDS:
   convert2lsif    Convert a SCIP index to an LSIF index
   cloc            Count a SCIP index's Lines of Code
   convert2cypher  Convert a SCIP index to memgrph...
   help, h         Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help (default: false)
   --version, -v  print the version (default: false)
```

In this tool, we partially referred to the implementation of the SCIP repository.


# Features and Concerns

## Complex Scenarios

**Question:** Could a publisher-subscriber pattern be recognized as connections? In another case, a message channel may only be established only when the sent data satisfies certain conditions required by the receiver.

**Answer:** We can recognize the publisher-subscriber pattern as connections. Because `protoc-gen-scip` only depends on the **name** of the proto definitions and the generated code. 
Please look at the [example code](https://github.com/chai2010/advanced-go-programming-book/tree/v1.0.0/examples/ch4.4/grpc-pubsub) that cherry picked on Github:

We define a proto file `pubsubservice.proto`.

```go
syntax = "proto3";

package pubsubservice;

message String {
	string value = 1;
}

service PubsubService {
	rpc Publish (String) returns (String);
	rpc Subscribe (String) returns (stream String);
}

```

The code generated by gRPC plugin should look like:

```go
type PubsubServiceServer interface {
    Publish(context.Context, *String) (*String, error)
    Subscribe(*String, PubsubService_SubscribeServer) error
}
type PubsubServiceClient interface {
    Publish(context.Context, *String, ...grpc.CallOption) (*String, error)
    Subscribe(context.Context, *String, ...grpc.CallOption) (
        PubsubService_SubscribeClient, error,
    )
}

type PubsubService_SubscribeServer interface {
    Send(*String) error
    grpc.ServerStream
}
```

At present, `protoc-gen-scip` facilitates the establishment of relationships between these specific interfaces and the proto definition. Leveraging the associated SCIP tools, we can seamlessly connect these symbols. This process is agnostic to specific use cases, provided it adheres to the paradigm established by the `protoc` compiler plugin.

## Possible Shortcommings

**Question:** Are there specifications for the fuzzy matcher algorithm? I'm not quite sure whether the name normalization and matching solution is precise enough.

**Answer:** Our fuzzy matcher hardcode the default naming convension of the popular used gRPC plugins. Which means `protoc-gen-scip` replies on the specific implementations of the plugin. If users hack the original gRPC plugins, the detection may fail due to the name changed.

**Solutions:** We expose a interface that allows user to define their own matcher. Since we consider the changement of plugin is not often, we think it is resonable to require some manual effort on this.

**Question:** Is there any false positives?

**Answer:**  In our implementation, we can not completely eliminate the
possibility of a false-positive when a user-defined type shares
the same names as the service and method names in a gRPC
definition. This occurrence is rare in practice, as it requires
an exact match for both the type name and method names.
However, even in these rare instances, our tool RPCover
remains robust - it won’t miss any gRPC calls that it should
identify. **Additionally, we provide an interface enabling users
to define their own matching logic, catering to the needs of
those using their own Protocol Buffer compiler plugins as we discussed above.**

# Reference

[SCIP](https://github.com/sourcegraph/scip/tree/main)

[LSIF](https://lsif.dev/)

[RPCoverBench](https://github.com/rpcover/RPCover)