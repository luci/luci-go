// Copyright 2018 The LUCI Authors.
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

package policy

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func TestRequestBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("FromCronTrigger", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Cron{},
		})
		assert.Loosely(t, r.Request, should.Resemble(task.Request{}))
	})

	ftt.Run("FromWebUITrigger", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Webui{},
		})
		assert.Loosely(t, r.Request, should.Resemble(task.Request{}))
	})

	ftt.Run("FromNoopTrigger", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Noop{Noop: &api.NoopTrigger{Data: "abc"}},
		})
		assert.Loosely(t, r.Request.Properties, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"noop_trigger_data": {
					Kind: &structpb.Value_StringValue{StringValue: "abc"},
				},
			},
		}))
		assert.Loosely(t, r.Request.Tags, should.BeEmpty)
	})

	ftt.Run("FromGitilesTrigger good", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Gitiles{Gitiles: &api.GitilesTrigger{
				Repo:     "https://example.googlesource.com/repo.git",
				Ref:      "refs/heads/master",
				Revision: "aaaaaaaa",
			}},
		})
		assert.Loosely(t, r.Request.Properties, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"revision": {
					Kind: &structpb.Value_StringValue{StringValue: "aaaaaaaa"},
				},
				"branch": {
					Kind: &structpb.Value_StringValue{StringValue: "refs/heads/master"},
				},
				"repository": {
					Kind: &structpb.Value_StringValue{StringValue: "https://example.googlesource.com/repo.git"},
				},
			},
		}))
		assert.Loosely(t, r.Request.Tags, should.Resemble([]string{
			"buildset:commit/gitiles/example.googlesource.com/repo/+/aaaaaaaa",
			"gitiles_ref:refs/heads/master",
		}))
	})

	ftt.Run("FromGitilesTrigger with extra properties and tags", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Gitiles{Gitiles: &api.GitilesTrigger{
				Repo:     "https://example.googlesource.com/repo.git",
				Ref:      "refs/heads/master",
				Revision: "aaaaaaaa",
				Properties: mergeIntoStruct(&structpb.Struct{}, map[string]string{
					"revision": "will-be-overridden",
					"stuff":    "remains",
				}),
				Tags: []string{
					"tag1:val1",
					"gitiles_ref:not-overridden",
					"tag1:val1", // dup
				},
			}},
		})
		assert.Loosely(t, r.Request.Properties, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"revision": {
					Kind: &structpb.Value_StringValue{StringValue: "aaaaaaaa"},
				},
				"branch": {
					Kind: &structpb.Value_StringValue{StringValue: "refs/heads/master"},
				},
				"repository": {
					Kind: &structpb.Value_StringValue{StringValue: "https://example.googlesource.com/repo.git"},
				},
				"stuff": {
					Kind: &structpb.Value_StringValue{StringValue: "remains"},
				},
			},
		}))
		assert.Loosely(t, r.Request.Tags, should.Resemble([]string{
			"buildset:commit/gitiles/example.googlesource.com/repo/+/aaaaaaaa",
			"gitiles_ref:refs/heads/master",
			"tag1:val1",
			"gitiles_ref:not-overridden",
		}))
	})

	ftt.Run("FromGitilesTrigger bad", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Gitiles{Gitiles: &api.GitilesTrigger{
				Repo:     "https://zzz.example.com/repo",
				Ref:      "refs/heads/master",
				Revision: "aaaaaaaa",
			}},
		})
		assert.Loosely(t, r.Request, should.Resemble(task.Request{
			DebugLog: "Bad repo URL \"https://zzz.example.com/repo\" in the trigger " +
				"- only .googlesource.com repos are supported\n",
		}))
	})

	ftt.Run("FromBuildbucketTrigger", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Buildbucket{Buildbucket: &api.BuildbucketTrigger{
				Properties: mergeIntoStruct(&structpb.Struct{}, map[string]string{"a": "b"}),
				Tags:       []string{"c:d"},
			}},
		})
		assert.Loosely(t, r.Request.Properties, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"a": {
					Kind: &structpb.Value_StringValue{StringValue: "b"},
				},
			},
		}))
		assert.Loosely(t, r.Request.Tags, should.Resemble([]string{"c:d"}))
	})

	ftt.Run("From unknown", t, func(t *ftt.Test) {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{})
		assert.Loosely(t, r.Request, should.Resemble(task.Request{
			DebugLog: "Unrecognized trigger payload of type <nil>, ignoring\n",
		}))
	})
}
