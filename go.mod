module go.chromium.org/luci

go 1.20

require (
	cloud.google.com/go/bigquery v1.49.0
	cloud.google.com/go/bigtable v1.18.1
	cloud.google.com/go/cloudtasks v1.10.0
	cloud.google.com/go/compute/metadata v0.2.3
	cloud.google.com/go/datastore v1.11.0
	cloud.google.com/go/errorreporting v0.3.0
	cloud.google.com/go/iam v1.0.0
	cloud.google.com/go/kms v1.10.1
	cloud.google.com/go/logging v1.7.0
	cloud.google.com/go/profiler v0.3.1
	cloud.google.com/go/pubsub v1.30.0
	cloud.google.com/go/secretmanager v1.10.0
	cloud.google.com/go/spanner v1.45.1
	cloud.google.com/go/storage v1.30.1
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.16.1
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator v0.40.1
	github.com/Microsoft/go-winio v0.6.1
	github.com/alecthomas/participle/v2 v2.0.0-alpha7
	github.com/alicebob/gopher-json v0.0.0-20200520072559-a9ecdc9d1d3a
	github.com/alicebob/miniredis/v2 v2.30.4
	github.com/bazelbuild/buildtools v0.0.0-20221004120235-7186f635531b
	github.com/bazelbuild/remote-apis v0.0.0-20230411132548-35aee1c4a425
	github.com/bazelbuild/remote-apis-sdks v0.0.0-20230511173731-022d76cc3978
	github.com/danjacques/gofslock v0.0.0-20230227034627-eebdc482a3f5
	github.com/dgraph-io/badger/v3 v3.2103.2
	github.com/dustin/go-humanize v1.0.1
	github.com/envoyproxy/protoc-gen-validate v1.0.2
	github.com/go-git/go-git/v5 v5.7.0
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.3
	github.com/gomodule/redigo v1.8.9
	github.com/google/go-cmp v0.5.9
	github.com/google/s2a-go v0.1.4
	github.com/google/tink/go v1.7.0
	github.com/google/uuid v1.3.0
	github.com/googleapis/gax-go/v2 v2.8.0
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75
	github.com/jordan-wright/email v4.0.1-0.20210109023952-943e75fe5223+incompatible
	github.com/julienschmidt/httprouter v1.3.0
	github.com/klauspost/compress v1.16.7
	github.com/luci/gtreap v0.0.0-20161228054646-35df89791e8f
	github.com/maruel/subcommands v1.1.1
	github.com/mattn/go-tty v0.0.5
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pmezard/go-difflib v1.0.0
	github.com/protocolbuffers/txtpbfmt v0.0.0-20230706122021-58d06d1b3926
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/sergi/go-diff v1.3.1
	github.com/smarty/assertions v1.15.1
	github.com/smartystreets/goconvey v1.8.1
	github.com/vmihailenco/msgpack/v5 v5.3.5
	github.com/yosuke-furukawa/json5 v0.1.1
	github.com/yuin/gopher-lua v1.1.0
	go.opencensus.io v0.24.0
	go.opentelemetry.io/contrib/detectors/gcp v1.17.0
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/trace v1.16.0
	go.starlark.net v0.0.0-20230612165344-9532f5667272
	golang.org/x/crypto v0.11.0
	golang.org/x/net v0.12.0
	golang.org/x/oauth2 v0.9.0
	golang.org/x/sync v0.3.0
	golang.org/x/sys v0.10.0
	golang.org/x/term v0.10.0
	golang.org/x/text v0.11.0
	golang.org/x/time v0.3.0
	golang.org/x/tools v0.11.0
	gonum.org/v1/gonum v0.12.0
	google.golang.org/api v0.118.0
	google.golang.org/appengine v1.6.8-0.20221117013220-504804fb50de
	google.golang.org/genproto v0.0.0-20230403163135-c38d8f061ccd
	google.golang.org/grpc v1.56.0-dev
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.3.0
	google.golang.org/protobuf v1.31.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	cloud.google.com/go v0.110.0 // indirect
	cloud.google.com/go/compute v1.19.1 // indirect
	cloud.google.com/go/longrunning v0.4.1 // indirect
	cloud.google.com/go/trace v1.9.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.13.1 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.40.1 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/apache/arrow/go/v11 v11.0.0 // indirect
	github.com/apache/thrift v0.16.0 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cncf/udpa/go v0.0.0-20220112060539-c52dc94e7fbe // indirect
	github.com/cncf/xds/go v0.0.0-20230607035331-e9ce68804cb4 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/envoyproxy/go-control-plane v0.11.1-0.20230524094728-9239064ad72f // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.4.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/goccy/go-json v0.9.11 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/flatbuffers v2.0.8+incompatible // indirect
	github.com/google/pprof v0.0.0-20221103000818-d260c55eee4c // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.5 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/iancoleman/strcase v0.2.0 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/asmfmt v1.3.2 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/lyft/protoc-gen-star/v2 v2.0.3 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/minio/asm2plan9s v0.0.0-20200509001527-cdd76441f9d8 // indirect
	github.com/minio/c2goasm v0.0.0-20190812172519-36a3d3bbc4f3 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mostynb/zstdpool-syncpool v0.0.12 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/xattr v0.4.7 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/texttheater/golang-levenshtein v1.0.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e
	github.com/zeebo/xxh3 v1.0.2 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
)
