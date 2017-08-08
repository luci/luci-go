proto-gae
=========

proto-gae is a simple `go generate`-compatible tool for generating
"go.chromium.org/gae/service/datastore".PropertyConverter implementation for
`protoc`-generated message types. This allows you to embed `proto.Message`
implementations into your datastore models.

The generated implementations serialize to/from the binary protobuf format into
an unindexed []byte property.


Example
-------

#### path/to/mything/protos/mything.proto
```protobuf
syntax = "proto3";
package protos;
message MyProtoThing {
  my string = 1;
  proto int64 = 1;
  thing float = 1;
}
```

#### path/to/mything/protos/gen.go
```go
package protos
// assume go.chromium.org/luci/tools/cmd/cproto is in $PATH. Try it, it's
// awesome :).

//go:generate cproto
//go:generate proto-gae -type MyProtoThing
```

#### path/to/mything/thing.go
```go
package mything

import "path/to/package/protos"

type DatastoreModel struct {
  // This will now 'just work'; ProtoMessage will round-trip to datastore as
  // []byte.
  ProtoMessage protos.MyProtoThing
}
```
