// ****WARNING****
// After modifying this file, a manual roll in infra.git might be needed.
// If so, run `go mod tidy` in:
//     https://chromium.googlesource.com/infra/infra/+/refs/heads/main/go/src/infra/
// Once the go.mod file in infra.git is synced up with this one, another manual roll
// might be needed for infra_internal.git.
// Similarly, run `go mod tidy` in:
//     https://chrome-internal.googlesource.com/infra/infra_internal/+/refs/heads/main/go/
// This is due to the dependency of luci-go => infra => infra_internal.
// This is not an exhaustive list of all downstream repos/ manual rolls, it is merely the ones
// the LUCI team is aware of and can make updates to.

module go.chromium.org/luci

go 1.22

require (
	cloud.google.com/go/bigquery v1.59.1
	cloud.google.com/go/bigtable v1.21.0
	cloud.google.com/go/cloudtasks v1.12.6
	cloud.google.com/go/compute/metadata v0.3.0
	cloud.google.com/go/datastore v1.15.0
	cloud.google.com/go/errorreporting v0.3.0
	cloud.google.com/go/iam v1.1.6
	cloud.google.com/go/kms v1.15.7
	cloud.google.com/go/logging v1.9.0
	cloud.google.com/go/profiler v0.4.0
	cloud.google.com/go/pubsub v1.36.1
	cloud.google.com/go/secretmanager v1.11.5
	cloud.google.com/go/spanner v1.57.0
	cloud.google.com/go/storage v1.38.0
	cloud.google.com/go/vertexai v0.7.1
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.21.0
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator v0.45.0
	github.com/Microsoft/go-winio v0.6.1
	github.com/alecthomas/participle/v2 v2.1.0
	github.com/alicebob/gopher-json v0.0.0-20200520072559-a9ecdc9d1d3a
	github.com/alicebob/miniredis/v2 v2.31.1
	github.com/apache/beam/sdks/v2 v2.54.0
	github.com/armon/go-radix v1.0.0
	github.com/bazelbuild/buildtools v0.0.0-20221004120235-7186f635531b
	github.com/bazelbuild/remote-apis v0.0.0-20240215191509-9ff14cecffe5
	github.com/bazelbuild/remote-apis-sdks v0.0.0-20240521192600-a5e60eb5cc47
	github.com/danjacques/gofslock v0.0.0-20240212154529-d899e02bfe22
	github.com/dave/dst v0.27.3
	github.com/dgraph-io/badger/v3 v3.2103.2
	github.com/dustin/go-humanize v1.0.1
	github.com/envoyproxy/protoc-gen-validate v1.0.4
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.4
	github.com/gomodule/redigo v1.8.9
	github.com/google/cel-go v0.20.1
	github.com/google/go-cmp v0.6.0
	github.com/google/s2a-go v0.1.7
	github.com/google/tink/go v1.7.0
	github.com/google/uuid v1.6.0
	github.com/googleapis/gax-go/v2 v2.12.1
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75
	github.com/jordan-wright/email v4.0.1-0.20210109023952-943e75fe5223+incompatible
	github.com/julienschmidt/httprouter v1.3.0
	github.com/klauspost/compress v1.17.8
	github.com/luci/gtreap v0.0.0-20161228054646-35df89791e8f
	github.com/maruel/subcommands v1.1.1
	github.com/mattn/go-tty v0.0.5
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pmezard/go-difflib v1.0.0
	github.com/protocolbuffers/txtpbfmt v0.0.0-20240116145035-ef3ab179eed6
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/sergi/go-diff v1.3.1
	github.com/smarty/assertions v1.15.1
	github.com/smartystreets/goconvey v1.8.1
	github.com/vmihailenco/msgpack/v5 v5.3.5
	github.com/yosuke-furukawa/json5 v0.1.1
	github.com/yuin/gopher-lua v1.1.0
	go.opentelemetry.io/contrib/detectors/gcp v1.23.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.48.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.47.0
	go.opentelemetry.io/otel v1.23.1
	go.opentelemetry.io/otel/sdk v1.23.1
	go.opentelemetry.io/otel/trace v1.23.1
	go.starlark.net v0.0.0-20240123142251-f86470692795
	golang.org/x/crypto v0.21.0
	golang.org/x/exp v0.0.0-20231006140011-7918f672742d
	golang.org/x/net v0.23.0
	golang.org/x/oauth2 v0.20.0
	golang.org/x/sync v0.6.0
	golang.org/x/sys v0.18.0
	golang.org/x/term v0.18.0
	golang.org/x/text v0.14.0
	golang.org/x/time v0.5.0
	golang.org/x/tools v0.18.0
	gonum.org/v1/gonum v0.12.0
	google.golang.org/api v0.164.0
	google.golang.org/appengine v1.6.8
	google.golang.org/genproto v0.0.0-20240213162025-012b6fc9bca9
	google.golang.org/genproto/googleapis/api v0.0.0-20240213162025-012b6fc9bca9
	google.golang.org/genproto/googleapis/bytestream v0.0.0-20240213162025-012b6fc9bca9
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240213162025-012b6fc9bca9
	google.golang.org/grpc v1.61.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.3.0
	google.golang.org/protobuf v1.33.0
	gopkg.in/yaml.v2 v2.4.0
)

require github.com/leemcloughlin/gofarmhash v0.0.0-20160919192320-0a055c5b87a8

require (
	cloud.google.com/go v0.112.0 // indirect
	cloud.google.com/go/aiplatform v1.60.0 // indirect
	cloud.google.com/go/longrunning v0.5.5 // indirect
	cloud.google.com/go/trace v1.10.5 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.21.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.45.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/apache/arrow/go/v14 v14.0.2 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cncf/udpa/go v0.0.0-20220112060539-c52dc94e7fbe // indirect
	github.com/cncf/xds/go v0.0.0-20231109132714-523115ebc101 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v24.0.7+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/envoyproxy/go-control-plane v0.11.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v23.5.26+incompatible // indirect
	github.com/google/pprof v0.0.0-20230602150820-91b7bce49751 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/lyft/protoc-gen-star/v2 v2.0.3 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc5 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/xattr v0.4.9 // indirect
	github.com/spf13/afero v1.10.0 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.einride.tech/aip v0.66.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.23.1
	golang.org/x/mod v0.15.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	gopkg.in/retry.v1 v1.0.3 // indirect
)
