// Copyright 2017 The LUCI Authors.
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

package notify

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"testing"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotify(t *testing.T) {
	Convey("Notify", t, func() {
		c := gaetesting.TestingContextWithAppID("luci-notify")
		c = clock.Set(c, testclock.New(testclock.TestRecentTimeUTC))
		c = gologger.StdConfig.Use(c)
		c = logging.SetLevel(c, logging.Debug)

		build := &Build{
			Build: buildbucketpb.Build{
				Id: 54,
				Builder: &buildbucketpb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				Status: buildbucketpb.Status_SUCCESS,
			},
		}

		// Put Project and EmailTemplate entities.
		project := &config.Project{Name: "chromium", Revision: "deadbeef"}
		templates := []*config.EmailTemplate{
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}}",
			},
			{
				ProjectKey:          datastore.KeyForObj(c, project),
				Name:                "non-default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed from non-default template",
				BodyHTMLTemplate:    "Build {{.Build.Id}} completed with status {{.Build.Status}} from non-default template",
			},
		}
		So(datastore.Put(c, project, templates), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("createEmailTasks", func() {
			emailNotify := []EmailNotify{
				{
					Email: "jane@example.com",
				},
				{
					Email: "john@example.com",
				},
				{
					Email:    "don@example.com",
					Template: "non-default",
				},
			}
			tasks, err := createEmailTasks(c, emailNotify, &notifypb.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build:               &build.Build,
				OldStatus:           buildbucketpb.Status_SUCCESS,
			})
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 3)

			t := tasks[0].Payload.(*internal.EmailTask)
			So(tasks[0].DeduplicationKey, ShouldEqual, "54-default-jane@example.com")
			So(t.Recipients, ShouldResemble, []string{"jane@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS")

			t = tasks[1].Payload.(*internal.EmailTask)
			So(tasks[1].DeduplicationKey, ShouldEqual, "54-default-john@example.com")
			So(t.Recipients, ShouldResemble, []string{"john@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS")

			t = tasks[2].Payload.(*internal.EmailTask)
			So(tasks[2].DeduplicationKey, ShouldEqual, "54-non-default-don@example.com")
			So(t.Recipients, ShouldResemble, []string{"don@example.com"})
			So(t.Subject, ShouldEqual, "Build 54 completed from non-default template")
			So(decompress(t.BodyGzip), ShouldEqual, "Build 54 completed with status SUCCESS from non-default template")
		})

		Convey("createEmailTasks with dup notifies", func() {
			emailNotify := []EmailNotify{
				{
					Email: "jane@example.com",
				},
				{
					Email: "jane@example.com",
				},
			}
			tasks, err := createEmailTasks(c, emailNotify, &notifypb.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build:               &build.Build,
				OldStatus:           buildbucketpb.Status_SUCCESS,
			})
			So(err, ShouldBeNil)
			So(tasks, ShouldHaveLength, 1)
		})
	})
}

func decompress(gzipped []byte) string {
	r, err := gzip.NewReader(bytes.NewReader(gzipped))
	So(err, ShouldBeNil)
	buf, err := ioutil.ReadAll(r)
	So(err, ShouldBeNil)
	return string(buf)
}
