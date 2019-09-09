module go.chromium.org/luci

go 1.12

require (
	cloud.google.com/go v0.44.3
	cloud.google.com/go/bigquery v1.0.1
	cloud.google.com/go/bigtable v1.0.0
	cloud.google.com/go/datastore v1.0.0
	cloud.google.com/go/logging v1.0.0 // indirect
	github.com/Masterminds/squirrel v1.1.0
	github.com/VividCortex/mysqlerr v0.0.0-20170204212430-6c6b55f8796f
	github.com/bradfitz/gomemcache v0.0.0-20190329173943-551aad21a668 // indirect
	github.com/danjacques/gofslock v0.0.0-20180405201223-afa47669cc54
	github.com/dustin/go-humanize v1.0.0
	github.com/go-sql-driver/mysql v1.4.1
	github.com/golang/mock v1.3.1
	github.com/golang/protobuf v1.3.2
	github.com/gomodule/redigo v2.0.1-0.20190822095346-d3876d43bbbe+incompatible
	github.com/google/uuid v1.1.1
	github.com/googleapis/gax-go v2.0.2+incompatible
	github.com/googleapis/gax-go/v2 v2.0.5
	github.com/gorhill/cronexpr v0.0.0-20180427100037-88b0669f7d75
	github.com/julienschmidt/httprouter v1.2.0
	github.com/kr/pretty v0.1.0
	github.com/luci/go-render v0.0.0-20160219211803-9a04cc21af0f
	github.com/luci/gtreap v0.0.0-20161228054646-35df89791e8f
	github.com/maruel/subcommands v0.0.0-20181220013616-967e945be48b
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b
	github.com/mitchellh/go-homedir v1.1.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pmezard/go-difflib v1.0.0
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/smartystreets/assertions v1.0.1
	github.com/smartystreets/goconvey v0.0.0-20190731233626-505e41936337
	github.com/texttheater/golang-levenshtein v0.0.0-20190717060638-b7aaf30637d6 // indirect
	github.com/xtgo/set v1.0.0
	github.com/yosuke-furukawa/json5 v0.1.1
	go.chromium.org/chromiumos/infra/proto/go v0.0.0-20190829195306-f93161233af6
	go.chromium.org/gae v0.0.0-20190826183307-50a499513efa
	go.starlark.net v0.0.0-20190820173200-988906f77f65
	golang.org/x/crypto v0.0.0-20190829043050-9756ffdc2472
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190830080133-08d80c9d36de
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/api v0.9.0
	google.golang.org/appengine v1.6.2
	google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc v1.23.0
	google.golang.org/protobuf v0.0.0-20190828194647-3cda377ed232
	gopkg.in/russross/blackfriday.v2 v2.0.0-00010101000000-000000000000
	gopkg.in/yaml.v2 v2.2.2
)

// https://github.com/russross/blackfriday/issues/558
replace gopkg.in/russross/blackfriday.v2 => github.com/russross/blackfriday/v2 v2.0.1
