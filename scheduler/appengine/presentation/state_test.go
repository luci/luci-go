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

package presentation

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/urlfetch"
)

func TestGetJobTraits(t *testing.T) {
	t.Parallel()
	Convey("works", t, func() {
		cat := catalog.New()
		So(cat.RegisterTaskManager(&urlfetch.TaskManager{}), ShouldBeNil)
		ctx := context.Background()

		Convey("bad task", func() {
			taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
				Noop: &messages.NoopTask{},
			})
			So(err, ShouldBeNil)
			traits, err := GetJobTraits(ctx, cat, &engine.Job{
				Task: taskBlob,
			})
			So(err, ShouldNotBeNil)
			So(traits, ShouldResemble, task.Traits{})
		})

		Convey("OK task", func() {
			taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
				UrlFetch: &messages.UrlFetchTask{Url: "http://example.com/path"},
			})
			So(err, ShouldBeNil)
			traits, err := GetJobTraits(ctx, cat, &engine.Job{
				Task: taskBlob,
			})
			So(err, ShouldBeNil)
			So(traits, ShouldResemble, task.Traits{})
		})
	})
}
