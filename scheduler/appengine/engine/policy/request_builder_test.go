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

	structpb "github.com/golang/protobuf/ptypes/struct"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRequestBuilder(t *testing.T) {
	t.Parallel()

	Convey("FromCronTrigger", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Cron{},
		})
		So(r.Request, ShouldResemble, task.Request{})
	})

	Convey("FromWebUITrigger", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Webui{},
		})
		So(r.Request, ShouldResemble, task.Request{})
	})

	Convey("FromNoopTrigger", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Noop{Noop: &api.NoopTrigger{Data: "abc"}},
		})
		So(r.Request.Properties, ShouldResembleProto, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"noop_trigger_data": {
					Kind: &structpb.Value_StringValue{StringValue: "abc"},
				},
			},
		})
		So(r.Request.Tags, ShouldBeEmpty)
	})

	Convey("FromGitilesTrigger good", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Gitiles{Gitiles: &api.GitilesTrigger{
				Repo:     "https://example.googlesource.com/repo.git",
				Ref:      "refs/heads/master",
				Revision: "aaaaaaaa",
			}},
		})
		So(r.Request.Properties, ShouldResembleProto, &structpb.Struct{
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
		})
		So(r.Request.Tags, ShouldResemble, []string{
			"buildset:commit/gitiles/example.googlesource.com/repo/+/aaaaaaaa",
			"buildset:commit/git/aaaaaaaa",
			"gitiles_ref:refs/heads/master",
		})
	})

	Convey("FromGitilesTrigger with extra properties and tags", t, func() {
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
					"tag1:val1",                    // dup
					"buildset:commit/git/aaaaaaaa", // dup
				},
			}},
		})
		So(r.Request.Properties, ShouldResembleProto, &structpb.Struct{
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
		})
		So(r.Request.Tags, ShouldResemble, []string{
			"buildset:commit/gitiles/example.googlesource.com/repo/+/aaaaaaaa",
			"buildset:commit/git/aaaaaaaa",
			"gitiles_ref:refs/heads/master",
			"tag1:val1",
			"gitiles_ref:not-overridden",
		})
	})

	Convey("FromGitilesTrigger bad", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Gitiles{Gitiles: &api.GitilesTrigger{
				Repo:     "https://zzz.example.com/repo",
				Ref:      "refs/heads/master",
				Revision: "aaaaaaaa",
			}},
		})
		So(r.Request, ShouldResemble, task.Request{
			DebugLog: "Bad repo URL \"https://zzz.example.com/repo\" in the trigger " +
				"- only .googlesource.com repos are supported\n",
		})
	})

	Convey("FromBuildbucketTrigger", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{
			Payload: &internal.Trigger_Buildbucket{Buildbucket: &api.BuildbucketTrigger{
				Properties: mergeIntoStruct(&structpb.Struct{}, map[string]string{"a": "b"}),
				Tags:       []string{"c:d"},
			}},
		})
		So(r.Request.Properties, ShouldResembleProto, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"a": {
					Kind: &structpb.Value_StringValue{StringValue: "b"},
				},
			},
		})
		So(r.Request.Tags, ShouldResemble, []string{"c:d"})
	})

	Convey("From unknown", t, func() {
		r := RequestBuilder{}
		r.FromTrigger(&internal.Trigger{})
		So(r.Request, ShouldResemble, task.Request{
			DebugLog: "Unrecognized trigger payload of type <nil>, ignoring\n",
		})
	})
}
