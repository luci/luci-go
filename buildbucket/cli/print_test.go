// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/mgutz/ansi"

	"go.chromium.org/luci/common/clock/testclock"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

var buildJSON = `
{
  "id": "8917899588926498064",
  "builder": {
    "project": "chromium",
    "bucket": "try",
    "builder": "linux-rel"
  },
  "number": 1,
  "createdBy": "user:5071639625-1lppvbtck1morgivc6sq4dul7klu27sd@developer.gserviceaccount.com",
  "createTime": "2019-03-26T18:33:47.958975Z",
  "startTime": "2019-03-26T18:33:52.447816Z",
  "endTime": "2019-03-26T18:37:13.892433Z",
  "updateTime": "2019-03-26T18:37:14.629154Z",
  "status": "SUCCESS",
  "summaryMarkdown": "it was ok",
  "input": {
    "experimental": true,
    "properties": {
      "$recipe_engine/cq": {
        "dry_run": true
      },
      "$recipe_engine/runtime": {
        "is_experimental": false,
        "is_luci": true
      },
      "blamelist": [
        "hinoka@chromium.org"
      ],
      "buildername": "Luci-go Mac Tester",
      "category": "cq"
    },
    "gitilesCommit": {
        "host": "chromium.googlesource.com",
        "project": "infra/luci/luci-go",
        "ref": "refs/heads/master",
        "id": "deadbeef"
    },
    "gerritChanges": [
      {
        "host": "chromium-review.googlesource.com",
        "project": "infra/luci/luci-go",
        "change": "1539021",
        "patchset": "1"
      }
    ]
  },
  "output": {
    "properties": {
      "$recipe_engine/cq": {
        "dry_run": true
      },
      "bot_id": "build234-m9"
    }
  },
  "steps": [
    {
      "name": "first",
      "startTime": "2019-03-26T18:34:01.896053Z",
      "endTime": "2019-03-26T18:34:01.954986Z",
      "status": "SUCCESS",
      "logs": [
        {
          "name": "stdout",
          "viewUrl": "https://logs.example.com/first/stdout"
        },
        {
          "name": "stderr",
          "viewUrl": "https://logs.example.com/first/stderr"
        }
      ],
      "summaryMarkdown": "first summary markdown\nsummary second line"
    },
    {
      "name": "second",
      "startTime": "2019-03-26T18:34:01.896053Z",
      "endTime": "2019-03-26T18:34:01.954986Z",
      "status": "SUCCESS",
      "logs": [
        {
          "name": "stdout",
          "viewUrl": "https://logs.example.com/second/stdout"
        },
        {
          "name": "stderr",
          "viewUrl": "https://logs.example.com/second/stderr"
        }
      ]
    }
  ],
  "infra": {
    "buildbucket": {
      "serviceConfigRevision": "80aefcf1d2f6c25894a968312eca86ba0f80737c",
      "canary": true
    },
    "swarming": {
      "hostname": "chromium-swarm.appspot.com",
      "taskId": "43d4192e94d9ff10",
      "taskServiceAccount": "infra-try-builder@chops-service-accounts.iam.gserviceaccount.com"
    },
    "logdog": {
      "hostname": "logs.chromium.org",
      "project": "infra",
      "prefix": "buildbucket/cr-buildbucket.appspot.com/8917899588926498064"
    }
  },
  "tags": [
    {
      "key": "buildset",
      "value": "patch/gerrit/chromium-review.googlesource.com/1539021/1"
    },
    {
      "key": "cq_experimental",
      "value": "false"
    },
    {
      "key": "user_agent",
      "value": "cq"
    }
  ]
}`

const expectedBuildPrintedTemplate = `<white+b><white+u><green+h>http://ci.chromium.org/b/8917899588926498064<reset><white+b><green+h> SUCCESS   'chromium/try/linux-rel/1'<reset>
<white+b>Summary<reset>: it was ok
<white+b>Experimental<reset> <white+b>Canary<reset>
<white+b>Created<reset> on 2019-03-26 at 18:33:47, <white+b>waited<reset> 4.5s, <white+b>started<reset> at 18:33:52, <white+b>ran<reset> for 3m21s, <white+b>ended<reset> at 18:37:13
<white+b>By<reset>: user:5071639625-1lppvbtck1morgivc6sq4dul7klu27sd@developer.gserviceaccount.com
<white+b>Commit<reset>: <white+u>https://crrev.com/deadbeef<reset> on refs/heads/master
<white+b>CL<reset>: <white+u>https://crrev.com/c/1539021/1<reset>
<white+b>Tag<reset>: buildset:patch/gerrit/chromium-review.googlesource.com/1539021/1
<white+b>Tag<reset>: cq_experimental:false
<white+b>Tag<reset>: user_agent:cq
<white+b>Input properties<reset>: {
  "$recipe_engine/cq": {
    "dry_run": true
  },
  "$recipe_engine/runtime": {
    "is_experimental": false,
    "is_luci": true
  },
  "blamelist": [
    "hinoka@chromium.org"
  ],
  "buildername": "Luci-go Mac Tester",
  "category": "cq"
}
<white+b>Output properties<reset>: {
  "$recipe_engine/cq": {
    "dry_run": true
  },
  "bot_id": "build234-m9"
}
<green+h>Step "first"    SUCCESS   59ms      Logs: "stdout", "stderr"<reset>
  first summary markdown
  summary second line
<green+h>Step "second"   SUCCESS   59ms      Logs: "stdout", "stderr"<reset>
`

func TestPrint(t *testing.T) {
	Convey("Print", t, func() {
		buf := &bytes.Buffer{}
		p := newPrinter(buf, false, func() time.Time {
			return testclock.TestRecentTimeUTC
		})

		Convey("Build", func() {
			build := &pb.Build{}
			So(jsonpb.UnmarshalString(buildJSON, build), ShouldBeNil)

			expectedBuildPrinted := regexp.MustCompile("<[^>]+>").ReplaceAllStringFunc(
				expectedBuildPrintedTemplate,
				func(style string) string {
					style = strings.TrimPrefix(style, "<")
					style = strings.TrimSuffix(style, ">")
					return ansi.ColorCode(style)
				})
			p.Build(build)

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, expectedBuildPrinted)
		})

		Convey("Chromium commit", func() {
			p.commit(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "refs/heads/master",
				Id:      "deadbeef",
			})

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, ansi.Color("https://crrev.com/deadbeef", "white+u")+" on refs/heads/master")
		})

		Convey("Chromium commit without id", func() {
			p.commit(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/heads/master",
			})

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, ansi.Color("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/master", "white+u"))
		})

		Convey("Arbitrary commit", func() {
			p.commit(&pb.GitilesCommit{
				Host:    "fuchsia.googlesource.com",
				Project: "infra",
				Ref:     "refs/heads/master",
				Id:      "deadbeef",
			})

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, ansi.Color("https://fuchsia.googlesource.com/infra/+/deadbeef", "white+u")+" on refs/heads/master")
		})

		Convey("Arbitrary commit without id", func() {
			p.commit(&pb.GitilesCommit{
				Host:    "fuchsia.googlesource.com",
				Project: "infra",
				Ref:     "refs/heads/master",
			})

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, ansi.Color("https://fuchsia.googlesource.com/infra/+/refs/heads/master", "white+u"))
		})

		Convey("Chromium CL", func() {
			p.change(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   123,
				Patchset: 4,
			})

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, ansi.Color("https://crrev.com/c/123/4", "white+u"))
		})

		Convey("Chrome CL", func() {
			p.change(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Project:  "secret",
				Change:   123,
				Patchset: 4,
			})

			So(p.Err, ShouldBeNil)
			So(buf.String(), ShouldEqual, ansi.Color("https://crrev.com/i/123/4", "white+u"))
		})
	})
}
