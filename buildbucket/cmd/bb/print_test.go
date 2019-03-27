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

package main

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/jsonpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

var buildJSON = `
{
  "id": "8917899588926498064",
  "builder": {
    "project": "infra",
    "bucket": "try",
    "builder": "Luci-go Mac Tester"
  },
  "createdBy": "user:5071639625-1lppvbtck1morgivc6sq4dul7klu27sd@developer.gserviceaccount.com",
  "createTime": "2019-03-26T18:33:47.958975Z",
  "startTime": "2019-03-26T18:33:52.447816Z",
  "endTime": "2019-03-26T18:37:13.892433Z",
  "updateTime": "2019-03-26T18:37:14.629154Z",
  "status": "SUCCESS",
  "summaryMarkdown": "it was ok",
  "input": {
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
      "name": "step-1",
      "startTime": "2019-03-26T18:34:01.896053Z",
      "endTime": "2019-03-26T18:34:01.954986Z",
      "status": "SUCCESS",
      "logs": [
        {
          "name": "stdout",
          "viewUrl": "https://logs.example.com/step-1/stdout"
        },
        {
          "name": "stderr",
          "viewUrl": "https://logs.example.com/step-1/stderr"
        }
      ],
      "summaryMarkdown": "step-1 summary markdown\nsummary second line"
    },
    {
      "name": "step-2",
      "startTime": "2019-03-26T18:34:01.896053Z",
      "endTime": "2019-03-26T18:34:01.954986Z",
      "status": "SUCCESS",
      "logs": [
        {
          "name": "stdout",
          "viewUrl": "https://logs.example.com/step-2/stdout"
        },
        {
          "name": "stderr",
          "viewUrl": "https://logs.example.com/step-2/stderr"
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

const expectedBuildPrinted = `ID: 8917899588926498064
Builder: infra/try/Luci-go Mac Tester
Status: SUCCESS it was ok
Created on 2019-03-26 at 11:33:47, started at 11:33:52, ended at 11:33:52
Commit: https://chromium.googlesource.com/infra/luci/luci-go/+/deadbeef on refs/heads/master
CL: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1539021/1
Tag: buildset:patch/gerrit/chromium-review.googlesource.com/1539021/1
Tag: cq_experimental:false
Tag: user_agent:cq
Input properties: {
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
Output properties: {
  "$recipe_engine/cq": {
    "dry_run": true
  },
  "bot_id": "build234-m9"
}
Step "step-1"    SUCCESS    58.933ms
  step-1 summary markdown
  summary second line
  * stdout https://logs.example.com/step-1/stdout
  * stderr https://logs.example.com/step-1/stderr
Step "step-2"    SUCCESS    58.933ms
  * stdout https://logs.example.com/step-2/stdout
  * stderr https://logs.example.com/step-2/stderr
`

func TestPrint(t *testing.T) {
	Convey("Print", t, func() {
		buf := &bytes.Buffer{}
		p := newPrinter(buf)

		Convey("Build", func() {
			build := &buildbucketpb.Build{}
			So(jsonpb.UnmarshalString(buildJSON, build), ShouldBeNil)

			p.Build(build)
			So(buf.String(), ShouldEqual, expectedBuildPrinted)
		})

		Convey("Commit without id", func() {
			p.commit(&buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/heads/master",
			})
			p.tab.Flush()
			So(buf.String(), ShouldEqual, "https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/master")
		})
	})
}
