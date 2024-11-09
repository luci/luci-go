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
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/urlfetch"
)

func TestGetJobTraits(t *testing.T) {
	t.Parallel()
	ftt.Run("works", t, func(t *ftt.Test) {
		cat := catalog.New()
		assert.Loosely(t, cat.RegisterTaskManager(&urlfetch.TaskManager{}), should.BeNil)
		ctx := context.Background()

		t.Run("bad task", func(t *ftt.Test) {
			taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
				Noop: &messages.NoopTask{},
			})
			assert.Loosely(t, err, should.BeNil)
			traits, err := GetJobTraits(ctx, cat, &engine.Job{
				Task: taskBlob,
			})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, traits, should.Resemble(task.Traits{}))
		})

		t.Run("OK task", func(t *ftt.Test) {
			taskBlob, err := proto.Marshal(&messages.TaskDefWrapper{
				UrlFetch: &messages.UrlFetchTask{Url: "http://example.com/path"},
			})
			assert.Loosely(t, err, should.BeNil)
			traits, err := GetJobTraits(ctx, cat, &engine.Job{
				Task: taskBlob,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, traits, should.Resemble(task.Traits{}))
		})
	})
}
