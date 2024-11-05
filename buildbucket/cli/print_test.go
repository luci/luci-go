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

	"github.com/golang/protobuf/proto"
	"github.com/mgutz/ansi"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
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
  "canary": true,
  "input": {
    "experimental": true,
		"experiments": ["luci.buildbucket.canary_software", "luci.non_production"],
    "properties": {
      "$recipe_engine/cq": {
        "dry_run": true
      },
      "$recipe_engine/runtime": {
        "is_experimental": false
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
        "ref": "refs/heads/x",
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
      "serviceConfigRevision": "80aefcf1d2f6c25894a968312eca86ba0f80737c"
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

const expectedBuildPrintedTemplate = `<white+b><white+u><green+h>http://cr-buildbucket.appspot.com/build/8917899588926498064<reset><white+b><green+h> SUCCESS   'chromium/try/linux-rel/1'<reset>
<white+b>Summary<reset>: it was ok
<white+b>Experimental<reset> <white+b>Canary<reset>
<white+b>Created<reset> on 2019-03-26 at 18:33:47, <white+b>waited<reset> 4.5s, <white+b>started<reset> at 18:33:52, <white+b>ran<reset> for 3m21s, <white+b>ended<reset> at 18:37:13
<white+b>By<reset>: user:5071639625-1lppvbtck1morgivc6sq4dul7klu27sd@developer.gserviceaccount.com
<white+b>Commit<reset>: <white+u>https://crrev.com/deadbeef<reset> on refs/heads/x
<white+b>CL<reset>: <white+u>https://crrev.com/c/1539021/1<reset>
<white+b>Tag<reset>: buildset:patch/gerrit/chromium-review.googlesource.com/1539021/1
<white+b>Tag<reset>: cq_experimental:false
<white+b>Tag<reset>: user_agent:cq
<white+b>Input properties<reset>: {
  "$recipe_engine/cq": {
    "dry_run": true
  },
  "$recipe_engine/runtime": {
    "is_experimental": false
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
<white+b>Experiments<reset>: 
  luci.buildbucket.canary_software
  luci.non_production
<green+h>Step "first"    SUCCESS   59ms      Logs: "stdout", "stderr"<reset>
  first summary markdown
  summary second line
<green+h>Step "second"   SUCCESS   59ms      Logs: "stdout", "stderr"<reset>
`

func ansifyTemplate(template string) string {
	return regexp.MustCompile("<[^>]+>").ReplaceAllStringFunc(
		template,
		func(style string) string {
			style = strings.TrimPrefix(style, "<")
			style = strings.TrimSuffix(style, ">")
			return ansi.ColorCode(style)
		})
}

func TestPrint(t *testing.T) {
	ftt.Run("Print", t, func(t *ftt.Test) {
		buf := &bytes.Buffer{}
		p := newPrinter(buf, false, func() time.Time {
			return testclock.TestRecentTimeUTC
		})

		t.Run("Build", func(t *ftt.Test) {
			build := &pb.Build{}
			assert.Loosely(t, protojson.Unmarshal([]byte(buildJSON), build), should.BeNil)

			expectedBuildPrinted := ansifyTemplate(expectedBuildPrintedTemplate)
			p.Build(build, "")

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(expectedBuildPrinted))
		})

		t.Run("Partial build", func(t *ftt.Test) {
			// Only Id, Status and Builder are available
			build := &pb.Build{
				Id:     8917899588926498064,
				Status: pb.Status_STARTED,
				Builder: &pb.BuilderID{
					Project: "chromium",
					Bucket:  "try",
					Builder: "linux-rel",
				},
			}
			expectedBuildPrinted := ansifyTemplate("<white+b><white+u><yellow+h>http://cr-buildbucket-dev.appspot.com/build/8917899588926498064<reset><white+b><yellow+h> STARTED   'chromium/try/linux-rel'<reset>\n")
			p.Build(build, "cr-buildbucket-dev.appspot.com")

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(expectedBuildPrinted))
		})

		t.Run("Build error when any of required fields is missing", func(t *ftt.Test) {
			validBuild := &pb.Build{
				Id:     8917899588926498064,
				Status: pb.Status_STARTED,
				Builder: &pb.BuilderID{
					Project: "chromium",
					Bucket:  "try",
					Builder: "linux-rel",
				},
			}
			// Make sure Build can be printed
			p.Build(validBuild, "")
			assert.Loosely(t, p.Err, should.BeNil)

			buildWithoutID := proto.Clone(validBuild).(*pb.Build)
			buildWithoutID.Id = 0
			assert.Loosely(t, func() { p.Build(buildWithoutID, "") }, should.Panic)

			buildWithoutStatus := proto.Clone(validBuild).(*pb.Build)
			buildWithoutStatus.Status = pb.Status_STATUS_UNSPECIFIED
			assert.Loosely(t, func() { p.Build(buildWithoutStatus, "") }, should.Panic)

			buildWithoutBuilder := proto.Clone(validBuild).(*pb.Build)
			buildWithoutBuilder.Builder = nil
			assert.Loosely(t, func() { p.Build(buildWithoutBuilder, "") }, should.Panic)
		})

		t.Run("Chromium commit", func(t *ftt.Test) {
			p.commit(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "refs/heads/x",
				Id:      "deadbeef",
			})

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(ansi.Color("https://crrev.com/deadbeef", "white+u")+" on refs/heads/x"))
		})

		t.Run("Chromium commit without id", func(t *ftt.Test) {
			p.commit(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/heads/x",
			})

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(ansi.Color("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/x", "white+u")))
		})

		t.Run("Arbitrary commit", func(t *ftt.Test) {
			p.commit(&pb.GitilesCommit{
				Host:    "fuchsia.googlesource.com",
				Project: "infra",
				Ref:     "refs/heads/x",
				Id:      "deadbeef",
			})

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(ansi.Color("https://fuchsia.googlesource.com/infra/+/deadbeef", "white+u")+" on refs/heads/x"))
		})

		t.Run("Arbitrary commit without id", func(t *ftt.Test) {
			p.commit(&pb.GitilesCommit{
				Host:    "fuchsia.googlesource.com",
				Project: "infra",
				Ref:     "refs/heads/x",
			})

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(ansi.Color("https://fuchsia.googlesource.com/infra/+/refs/heads/x", "white+u")))
		})

		t.Run("Chromium CL", func(t *ftt.Test) {
			p.change(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   123,
				Patchset: 4,
			})

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(ansi.Color("https://crrev.com/c/123/4", "white+u")))
		})

		t.Run("Chrome CL", func(t *ftt.Test) {
			p.change(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Project:  "secret",
				Change:   123,
				Patchset: 4,
			})

			assert.Loosely(t, p.Err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(ansi.Color("https://crrev.com/i/123/4", "white+u")))
		})
	})
}
