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

go 1.23

// Don't add a `toolchain` directive here. We manage the version of our Go
// toolchain with a different mechanism.

require (
	cloud.google.com/go/bigquery v1.65.0
	cloud.google.com/go/bigtable v1.34.0
	cloud.google.com/go/cloudtasks v1.13.3
	cloud.google.com/go/compute/metadata v0.6.0
	cloud.google.com/go/datastore v1.20.0
	cloud.google.com/go/errorreporting v0.3.2
	cloud.google.com/go/iam v1.3.1
	cloud.google.com/go/kms v1.20.5
	cloud.google.com/go/logging v1.13.0
	cloud.google.com/go/profiler v0.4.2
	cloud.google.com/go/pubsub v1.45.3
	cloud.google.com/go/secretmanager v1.14.3
	cloud.google.com/go/spanner v1.73.0
	cloud.google.com/go/storage v1.50.0
	cloud.google.com/go/vertexai v0.7.1
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.25.0
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator v0.49.0
	github.com/Microsoft/go-winio v0.6.2
	github.com/alecthomas/participle/v2 v2.1.0
	github.com/alicebob/gopher-json v0.0.0-20230218143504-906a9b012302
	github.com/alicebob/miniredis/v2 v2.34.0
	github.com/armon/go-radix v1.0.0
	github.com/bazelbuild/buildtools v0.0.0-20221004120235-7186f635531b
	github.com/bazelbuild/remote-apis v0.0.0-20240926071355-6777112ef7de
	github.com/bazelbuild/remote-apis-sdks v0.0.0-20240910213405-f4821a2a072c
	github.com/bmatcuk/doublestar v1.3.4
	github.com/danjacques/gofslock v0.0.0-20240212154529-d899e02bfe22
	github.com/dgraph-io/badger/v3 v3.2103.2
	github.com/dustin/go-humanize v1.0.1
	github.com/envoyproxy/protoc-gen-validate v1.1.0
	github.com/go-git/go-git/v5 v5.13.1
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.4
	github.com/gomodule/redigo v1.9.2
	github.com/google/cel-go v0.20.1
	github.com/google/go-cmp v0.6.0
	github.com/google/s2a-go v0.1.9
	github.com/google/tink/go v1.7.0
	github.com/google/uuid v1.6.0
	github.com/googleapis/gax-go/v2 v2.14.1
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75
	github.com/jordan-wright/email v4.0.1-0.20210109023952-943e75fe5223+incompatible
	github.com/julienschmidt/httprouter v1.3.0
	github.com/klauspost/compress v1.17.11
	github.com/leemcloughlin/gofarmhash v0.0.0-20160919192320-0a055c5b87a8
	github.com/luci/gtreap v0.0.0-20161228054646-35df89791e8f
	github.com/maruel/subcommands v1.1.1
	github.com/mattn/go-tty v0.0.7
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pmezard/go-difflib v1.0.0
	github.com/protocolbuffers/txtpbfmt v0.0.0-20241112170944-20d2c9ebc01d
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3
	github.com/smarty/assertions v1.16.0
	github.com/vmihailenco/msgpack/v5 v5.3.5
	github.com/yosuke-furukawa/json5 v0.1.1
	github.com/yuin/gopher-lua v1.1.1
	go.opentelemetry.io/contrib/detectors/gcp v1.33.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.58.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.58.0
	go.opentelemetry.io/otel v1.33.0
	go.opentelemetry.io/otel/metric v1.33.0
	go.opentelemetry.io/otel/sdk v1.33.0
	go.opentelemetry.io/otel/trace v1.33.0
	go.starlark.net v0.0.0-20241226192728-8dfa5b98479f
	golang.org/x/crypto v0.32.0
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56
	golang.org/x/net v0.34.0
	golang.org/x/oauth2 v0.25.0
	golang.org/x/sync v0.10.0
	golang.org/x/sys v0.29.0
	golang.org/x/term v0.28.0
	golang.org/x/text v0.21.0
	golang.org/x/time v0.9.0
	golang.org/x/tools v0.29.0
	gonum.org/v1/gonum v0.12.0
	google.golang.org/api v0.217.0
	google.golang.org/appengine v1.6.8
	google.golang.org/genproto v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/genproto/googleapis/api v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/genproto/googleapis/bytestream v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/grpc v1.69.4
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.5.1
	google.golang.org/protobuf v1.36.3
	gopkg.in/yaml.v2 v2.4.0
	honnef.co/go/tools v0.1.3
)

require (
	cel.dev/expr v0.16.2 // indirect
	cloud.google.com/go v0.118.0 // indirect
	cloud.google.com/go/aiplatform v1.70.0 // indirect
	cloud.google.com/go/auth v0.14.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.7 // indirect
	cloud.google.com/go/longrunning v0.6.4 // indirect
	cloud.google.com/go/monitoring v1.22.1 // indirect
	cloud.google.com/go/trace v1.11.3 // indirect
	dario.cat/mergo v1.0.0 // indirect
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp v1.5.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.25.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.48.1 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.49.0 // indirect
	github.com/ProtonMail/go-crypto v1.1.3 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/apache/arrow/go/v15 v15.0.2 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/cncf/xds/go v0.0.0-20240905190251-b4127c9b8d78 // indirect
	github.com/cyphar/filepath-securejoin v0.3.6 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/envoyproxy/go-control-plane v0.13.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v23.5.26+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/lyft/protoc-gen-star/v2 v2.0.4-0.20230330145011-496ad1ac90a4 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/xattr v0.4.9 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/spf13/afero v1.10.0 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.einride.tech/aip v0.68.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.33.0 // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)
