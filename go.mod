module go.chromium.org/luci

go 1.16

require (
	cloud.google.com/go v0.90.0
	cloud.google.com/go/bigquery v1.20.0
	cloud.google.com/go/bigtable v1.10.1
	cloud.google.com/go/datastore v1.5.0
	cloud.google.com/go/logging v1.4.2
	cloud.google.com/go/pubsub v1.13.0
	cloud.google.com/go/spanner v1.24.0
	cloud.google.com/go/storage v1.16.0
	contrib.go.opencensus.io/exporter/stackdriver v0.13.8
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/Masterminds/squirrel v1.5.0
	github.com/Microsoft/go-winio v0.5.0
	github.com/VividCortex/mysqlerr v1.0.0
	github.com/aws/aws-sdk-go v1.40.15 // indirect
	github.com/bazelbuild/buildtools v0.0.0-20210804161516-27a6414354c1
	github.com/bazelbuild/remote-apis v0.0.0-20210718193713-0ecef08215cf
	github.com/bazelbuild/remote-apis-sdks v0.0.0-20210811083801-29944c168d49
	github.com/danjacques/gofslock v0.0.0-20200623023034-5d0bd0fa6ef0
	github.com/dgraph-io/badger/v3 v3.2103.1
	github.com/dustin/go-humanize v1.0.0
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang/glog v0.0.0-20210429001901-424d2337a529 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gomodule/redigo v1.8.5
	github.com/google/flatbuffers v2.0.0+incompatible // indirect
	github.com/google/go-cmp v0.5.6
	github.com/google/pprof v0.0.0-20210804190019-f964ff605595 // indirect
	github.com/google/tink/go v1.6.1
	github.com/google/uuid v1.3.0
	github.com/googleapis/gax-go/v2 v2.0.5
	github.com/gopherjs/gopherjs v0.0.0-20210803090616-8f023c250c89 // indirect
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75
	github.com/jordan-wright/email v4.0.1-0.20210109023952-943e75fe5223+incompatible
	github.com/julienschmidt/httprouter v1.3.0
	github.com/klauspost/compress v1.13.3
	github.com/kr/pretty v0.3.0
	github.com/luci/gtreap v0.0.0-20161228054646-35df89791e8f
	github.com/maruel/subcommands v1.1.0
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/mattn/go-isatty v0.0.13 // indirect
	github.com/mattn/go-tty v0.0.3
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mostynb/zstdpool-syncpool v0.0.8 // indirect
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pmezard/go-difflib v1.0.0
	github.com/protocolbuffers/txtpbfmt v0.0.0-20210726093723-1671b78f4579
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/sergi/go-diff v1.2.0
	github.com/smartystreets/assertions v1.2.0
	github.com/smartystreets/goconvey v1.6.4
	github.com/xtgo/set v1.0.0
	github.com/yosuke-furukawa/json5 v0.1.1
	go.opencensus.io v0.23.0
	go.starlark.net v0.0.0-20210602144842-1cdb82c9e17a
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	golang.org/x/tools v0.1.5
	google.golang.org/api v0.52.0
	google.golang.org/appengine v1.6.7
	google.golang.org/genproto v0.0.0-20210803142424-70bd63adacf2
	google.golang.org/grpc v1.39.0
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.1.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v2 v2.4.0
)

// The next version uses errors.Is(...) and no longer works on GAE go113.
replace golang.org/x/net => golang.org/x/net v0.0.0-20210503060351-7fd8e65b6420
