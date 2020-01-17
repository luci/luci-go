module go.chromium.org/luci

go 1.12

require (
	cloud.google.com/go v0.51.0
	cloud.google.com/go/bigquery v1.0.1
	cloud.google.com/go/bigtable v1.0.0
	cloud.google.com/go/datastore v1.0.0
	cloud.google.com/go/logging v1.0.0 // indirect
	cloud.google.com/go/pubsub v1.1.0
	cloud.google.com/go/spanner v1.1.0
	cloud.google.com/go/storage v1.0.0
	contrib.go.opencensus.io/exporter/stackdriver v0.12.8
	github.com/DATA-DOG/go-sqlmock v1.4.0
	github.com/Masterminds/squirrel v1.1.0
	github.com/Microsoft/go-winio v0.4.14
	github.com/VividCortex/mysqlerr v0.0.0-20170204212430-6c6b55f8796f
	github.com/bradfitz/gomemcache v0.0.0-20190913173617-a41fca850d0b // indirect
	github.com/danjacques/gofslock v0.0.0-20180405201223-afa47669cc54
	github.com/dustin/go-humanize v1.0.0
	github.com/go-sql-driver/mysql v1.4.1
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/gomodule/redigo v2.0.1-0.20190822095346-d3876d43bbbe+incompatible
	github.com/google/go-cmp v0.3.2-0.20191028172631-481baca67f93
	github.com/google/uuid v1.1.1
	github.com/googleapis/gax-go/v2 v2.0.5
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75
	github.com/julienschmidt/httprouter v1.2.0
	github.com/klauspost/compress v1.9.7
	github.com/kr/pretty v0.1.0
	github.com/lib/pq v1.3.0 // indirect
	github.com/luci/go-render v0.0.0-20160219211803-9a04cc21af0f
	github.com/luci/gtreap v0.0.0-20161228054646-35df89791e8f
	github.com/maruel/subcommands v0.0.0-20181220013616-967e945be48b
	github.com/maruel/ut v1.0.1 // indirect
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-sqlite3 v1.11.0 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b
	github.com/mitchellh/go-homedir v1.1.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pmezard/go-difflib v1.0.0
	github.com/sergi/go-diff v1.1.0
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/smartystreets/assertions v1.0.1
	github.com/smartystreets/goconvey v0.0.0-20190731233626-505e41936337
	github.com/texttheater/golang-levenshtein v0.0.0-20190717060638-b7aaf30637d6 // indirect
	github.com/xtgo/set v1.0.0
	github.com/yosuke-furukawa/json5 v0.1.1
	go.chromium.org/gae v0.0.0-20190826183307-50a499513efa
	go.opencensus.io v0.22.2
	go.starlark.net v0.0.0-20190820173200-988906f77f65
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	golang.org/x/net v0.0.0-20200114155413-6afb5195e5aa
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200113162924-86b910548bc1
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	google.golang.org/api v0.15.0
	google.golang.org/appengine v1.6.5
	google.golang.org/genproto v0.0.0-20200113173426-e1de0a7b01eb
	google.golang.org/grpc v1.26.0
	google.golang.org/protobuf v0.0.0-20190828194647-3cda377ed232
	gopkg.in/russross/blackfriday.v2 v2.0.0-00010101000000-000000000000
	gopkg.in/yaml.v2 v2.2.4
)

// https://github.com/russross/blackfriday/issues/558
replace gopkg.in/russross/blackfriday.v2 => github.com/russross/blackfriday/v2 v2.0.1
