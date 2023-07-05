## protoc-gen-scip

To test the plugin, execute the below command:

```shell
$ protoc --go_out=PROTO_OUT_PATH --scip_out=SCIP_OUT_PATH --plugin=./protoc-gen-scip --scip_opt=scip_file=INPUT_SCIP_INDEX PATH_TO_PROTO_SOURCE
```
