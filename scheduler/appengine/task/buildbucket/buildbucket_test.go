// Copyright 2015 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/engine/policy"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var _ task.Manager = (*TaskManager)(nil)

func TestValidateProtoMessage(t *testing.T) {
	t.Parallel()

	tm := TaskManager{}
	c := context.Background()

	Convey("ValidateProtoMessage works", t, func() {
		ctx := &validation.Context{Context: c}
		validate := func(msg proto.Message) error {
			tm.ValidateProtoMessage(ctx, msg, "some-project:some-realm")
			return ctx.Finalize()
		}

		Convey("ValidateProtoMessage passes good msg", func() {
			So(validate(&messages.BuildbucketTask{
				Server:     "blah.com",
				Bucket:     "bucket",
				Builder:    "builder",
				Tags:       []string{"a:b", "c:d"},
				Properties: []string{"a:b", "c:d"},
			}), ShouldBeNil)
		})

		Convey("ValidateProtoMessage passes good minimal msg", func() {
			So(validate(&messages.BuildbucketTask{
				Server:  "blah.com",
				Builder: "builder",
			}), ShouldBeNil)
		})

		Convey("ValidateProtoMessage wrong type", func() {
			So(validate(&messages.NoopTask{}), ShouldErrLike, "wrong type")
		})

		Convey("ValidateProtoMessage empty", func() {
			So(validate(tm.ProtoMessageType()), ShouldErrLike, "expecting a non-empty BuildbucketTask")
		})

		Convey("ValidateProtoMessage validates URL", func() {
			call := func(url string) error {
				ctx = &validation.Context{Context: c}
				tm.ValidateProtoMessage(ctx, &messages.BuildbucketTask{
					Server:  url,
					Bucket:  "bucket",
					Builder: "builder",
				}, "some-project:some-realm")
				return ctx.Finalize()
			}
			So(call(""), ShouldErrLike, "field 'server' is required")
			So(call("https://host/not-root"), ShouldErrLike, "field 'server' should be just a host, not a URL")
			So(call("%%%%"), ShouldErrLike, "field 'server' is not a valid hostname")
			So(call("blah.com/abc"), ShouldErrLike, "field 'server' is not a valid hostname")
		})

		Convey("ValidateProtoMessage needs bucket", func() {
			tm.ValidateProtoMessage(ctx, &messages.BuildbucketTask{
				Server:  "blah.com",
				Builder: "builder",
			}, "some-project:@legacy")
			So(ctx.Finalize(), ShouldErrLike, `'bucket' field for jobs in "@legacy" realm is required`)
		})

		Convey("ValidateProtoMessage needs builder", func() {
			So(validate(&messages.BuildbucketTask{
				Server: "blah.com",
				Bucket: "bucket",
			}), ShouldErrLike, "'builder' field is required")
		})

		Convey("ValidateProtoMessage validates properties", func() {
			So(validate(&messages.BuildbucketTask{
				Server:     "blah.com",
				Bucket:     "bucket",
				Builder:    "builder",
				Properties: []string{"not_kv_pair"},
			}), ShouldErrLike, "bad property, not a 'key:value' pair")
		})

		Convey("ValidateProtoMessage validates tags", func() {
			So(validate(&messages.BuildbucketTask{
				Server:  "blah.com",
				Bucket:  "bucket",
				Builder: "builder",
				Tags:    []string{"not_kv_pair"},
			}), ShouldErrLike, "bad tag, not a 'key:value' pair")
		})

		Convey("ValidateProtoMessage forbids default tags overwrite", func() {
			So(validate(&messages.BuildbucketTask{
				Server:  "blah.com",
				Bucket:  "bucket",
				Builder: "builder",
				Tags:    []string{"scheduler_job_id:blah"},
			}), ShouldErrLike, "tag \"scheduler_job_id\" is reserved")
		})
	})
}

func fakeController(testSrvURL string) *tasktest.TestController {
	return &tasktest.TestController{
		TaskMessage: &messages.BuildbucketTask{
			Server:  testSrvURL,
			Bucket:  "test-bucket",
			Builder: "builder",
			Tags:    []string{"a:from-task-def", "b:from-task-def"},
		},
		Req: task.Request{
			IncomingTriggers: []*internal.Trigger{
				{
					Id:    "trigger",
					Title: "Trigger",
					Url:   "https://trigger.example.com",
					Payload: &internal.Trigger_Gitiles{
						Gitiles: &api.GitilesTrigger{
							Repo:     "https://chromium.googlesource.com/chromium/src",
							Ref:      "refs/heads/master",
							Revision: "deadbeef",
						},
					},
				},
			},
		},
		Client:       http.DefaultClient,
		SaveCallback: func() error { return nil },
		PrepareTopicCallback: func(publisher string) (string, string, error) {
			if publisher != testSrvURL {
				panic(fmt.Sprintf("expecting %q, got %q", testSrvURL, publisher))
			}
			return "topic", "auth_token", nil
		},
	}
}

func TestBuilderID(t *testing.T) {
	t.Parallel()

	var cases = []struct {
		RealmID string
		Bucket  string
		Output  string
		Error   string
	}{
		{"proj:realm", "", "proj:realm", ""},
		{"proj:@legacy", "", "", "is required"},
		{"proj:@root", "", "", "is required"},

		{"proj:realm", "another-proj:buck", "another-proj:buck", ""},
		{"proj:realm", "buck", "proj:buck", ""},
		{"proj:realm", "abc.def.123", "proj:abc.def.123", ""},
		{"proj:@legacy", "buck", "proj:buck", ""},

		{"proj:realm", "luci.proj.buck", "", `use "buck" instead`},
		{"proj:realm", "luci.another-proj.buck", "", `use "another-proj:buck" instead`},
		{"proj:realm", "luci.another-proj", "", "need 3 components"},
	}

	for _, c := range cases {
		bid, err := builderID(&messages.BuildbucketTask{
			Bucket:  c.Bucket,
			Builder: "some-builder",
		}, c.RealmID)
		if c.Error != "" {
			if err == nil {
				t.Errorf("Expected to fail for %q %q, but did not", c.Bucket, c.RealmID)
			} else if !strings.Contains(err.Error(), c.Error) {
				t.Errorf("Expected to fail with %q, but failed with %q", c.Error, err.Error())
			}
		} else {
			if err != nil {
				t.Errorf("Expected to succeed for %q %q, but failed with %q", c.Bucket, c.RealmID, err.Error())
			} else if got := fmt.Sprintf("%s:%s", bid.Project, bid.Bucket); got != c.Output {
				t.Errorf("Expected to get %q, but got %q", c.Output, got)
			}
		}
	}
}

func TestFullFlow(t *testing.T) {
	t.Parallel()

	Convey("LaunchTask and HandleNotification work", t, func(ctx C) {
		scheduleRequest := make(chan *bbpb.ScheduleBuildRequest, 1)

		buildStatus := atomic.Value{}
		buildStatus.Store(bbpb.Status_STARTED)

		srv := BuildbucketFake{
			ScheduleBuild: func(req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
				scheduleRequest <- req
				return &bbpb.Build{
					Id:     9025781602559305888,
					Status: bbpb.Status_STARTED,
				}, nil
			},
			GetBuild: func(req *bbpb.GetBuildRequest) (*bbpb.Build, error) {
				if req.Id != 9025781602559305888 {
					return nil, status.Errorf(codes.NotFound, "wrong build ID")
				}
				return &bbpb.Build{
					Id:     req.Id,
					Status: buildStatus.Load().(bbpb.Status),
				}, nil
			},
		}
		srv.Start()
		defer srv.Stop()

		c := memory.Use(context.Background())
		c = mathrand.Set(c, rand.New(rand.NewSource(1000)))
		mgr := TaskManager{}
		ctl := fakeController(srv.URL())

		// Launch.
		So(mgr.LaunchTask(c, ctl), ShouldBeNil)
		So(ctl.TaskState, ShouldResemble, task.State{
			Status:   task.StatusRunning,
			TaskData: []byte(`{"build_id":"9025781602559305888"}`),
			ViewURL:  srv.URL() + "/build/9025781602559305888",
		})

		So(<-scheduleRequest, ShouldResembleProto, &bbpb.ScheduleBuildRequest{
			RequestId: "1",
			Builder: &bbpb.BuilderID{
				Project: "some-project",
				Bucket:  "test-bucket",
				Builder: "builder",
			},
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "deadbeef",
				Ref:     "refs/heads/master",
			},
			Properties: structFromJSON(`{
				"$recipe_engine/scheduler": {
					"hostname": "app.example.com",
					"job": "some-project/some-job",
					"invocation": "1",
					"triggers": [
						{
							"id": "trigger",
							"title": "Trigger",
							"url": "https://trigger.example.com",
							"gitiles": {
								"repo":     "https://chromium.googlesource.com/chromium/src",
								"ref":      "refs/heads/master",
								"revision": "deadbeef"
							}
						}
					]
				}
			}`),
			Tags: []*bbpb.StringPair{
				{Key: "scheduler_invocation_id", Value: "1"},
				{Key: "scheduler_job_id", Value: "some-project/some-job"},
				{Key: "user_agent", Value: "app"},
				{Key: "a", Value: "from-task-def"},
				{Key: "b", Value: "from-task-def"},
			},
			Notify: &bbpb.NotificationConfig{
				PubsubTopic: "topic",
				UserData:    []byte("auth_token"),
			},
		})

		// Added the timer.
		So(ctl.Timers, ShouldResemble, []tasktest.TimerSpec{
			{
				Delay: 224 * time.Second, // random
				Name:  statusCheckTimerName,
			},
		})
		ctl.Timers = nil

		// The timer is called. Checks the state, reschedules itself.
		So(mgr.HandleTimer(c, ctl, statusCheckTimerName, nil), ShouldBeNil)
		So(ctl.Timers, ShouldResemble, []tasktest.TimerSpec{
			{
				Delay: 157 * time.Second, // random
				Name:  statusCheckTimerName,
			},
		})

		// Process finish notification.
		buildStatus.Store(bbpb.Status_SUCCESS)
		So(mgr.HandleNotification(c, ctl, &pubsub.PubsubMessage{}), ShouldBeNil)
		So(ctl.TaskState.Status, ShouldEqual, task.StatusSucceeded)
	})
}

func TestAbort(t *testing.T) {
	t.Parallel()

	Convey("LaunchTask and AbortTask work", t, func(ctx C) {
		srv := BuildbucketFake{
			ScheduleBuild: func(req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
				return &bbpb.Build{
					Id:     9025781602559305888,
					Status: bbpb.Status_STARTED,
				}, nil
			},
			CancelBuild: func(req *bbpb.CancelBuildRequest) (*bbpb.Build, error) {
				if req.Id != 9025781602559305888 {
					return nil, status.Errorf(codes.NotFound, "wrong build ID")
				}
				return &bbpb.Build{
					Id:     req.Id,
					Status: bbpb.Status_CANCELED,
				}, nil
			},
		}
		srv.Start()
		defer srv.Stop()

		c := memory.Use(context.Background())
		mgr := TaskManager{}
		ctl := fakeController(srv.URL())

		// Launch and kill.
		So(mgr.LaunchTask(c, ctl), ShouldBeNil)
		So(mgr.AbortTask(c, ctl), ShouldBeNil)
	})
}

func TestTriggeredFlow(t *testing.T) {
	t.Parallel()

	Convey("LaunchTask with GitilesTrigger works", t, func(ctx C) {
		scheduleRequest := make(chan *bbpb.ScheduleBuildRequest, 1)

		srv := BuildbucketFake{
			ScheduleBuild: func(req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
				scheduleRequest <- req
				return &bbpb.Build{
					Id:     9025781602559305888,
					Status: bbpb.Status_STARTED,
				}, nil
			},
			GetBuild: func(req *bbpb.GetBuildRequest) (*bbpb.Build, error) {
				if req.Id != 9025781602559305888 {
					return nil, status.Errorf(codes.NotFound, "wrong build ID")
				}
				return &bbpb.Build{
					Id:     req.Id,
					Status: bbpb.Status_SUCCESS,
				}, nil
			},
		}
		srv.Start()
		defer srv.Stop()

		c := memory.Use(context.Background())
		mgr := TaskManager{}
		ctl := fakeController(srv.URL())

		schedule := func(triggers []*internal.Trigger) *bbpb.ScheduleBuildRequest {
			// Prepare the request the same way the engine does using RequestBuilder.
			req := policy.RequestBuilder{}
			req.FromTrigger(triggers[len(triggers)-1])
			req.IncomingTriggers = triggers
			ctl.Req = req.Request

			// Launch with triggers,
			So(mgr.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.TaskState, ShouldResemble, task.State{
				Status:   task.StatusRunning,
				TaskData: []byte(`{"build_id":"9025781602559305888"}`),
				ViewURL:  srv.URL() + "/build/9025781602559305888",
			})

			return <-scheduleRequest
		}

		Convey("Gitiles triggers", func() {
			req := schedule([]*internal.Trigger{
				{
					Id: "1",
					Payload: &internal.Trigger_Gitiles{
						Gitiles: &api.GitilesTrigger{
							Repo:     "https://r.googlesource.com/repo",
							Ref:      "refs/heads/master",
							Revision: "baadcafe",
						},
					},
				},
				{
					Id: "2",
					Payload: &internal.Trigger_Gitiles{
						Gitiles: &api.GitilesTrigger{
							Repo:       "https://r.googlesource.com/repo",
							Ref:        "refs/heads/master",
							Revision:   "deadbeef",
							Tags:       []string{"extra:tag", "gitiles_ref:refs/heads/master"},
							Properties: structFromJSON(`{"extra_prop": "val", "branch": "ignored"}`),
						},
					},
				},
			})

			// Used the last trigger to get the commit.
			So(req.GitilesCommit, ShouldResembleProto, &bbpb.GitilesCommit{
				Host:    "r.googlesource.com",
				Project: "repo",
				Id:      "deadbeef",
				Ref:     "refs/heads/master",
			})

			// Properties are sanitized.
			So(structKeys(req.Properties), ShouldResemble, []string{
				"$recipe_engine/scheduler",
				"extra_prop",
			})

			// Tags are sanitized too.
			So(req.Tags, ShouldResembleProto, []*bbpb.StringPair{
				{Key: "scheduler_invocation_id", Value: "1"},
				{Key: "scheduler_job_id", Value: "some-project/some-job"},
				{Key: "user_agent", Value: "app"},
				{Key: "a", Value: "from-task-def"},
				{Key: "b", Value: "from-task-def"},
				{Key: "extra", Value: "tag"},
			})
		})

		Convey("Reconstructs gitiles commit from generic trigger", func() {
			req := schedule([]*internal.Trigger{
				{
					Id: "1",
					Payload: &internal.Trigger_Buildbucket{
						Buildbucket: &api.BuildbucketTrigger{
							Properties: structFromJSON(`{
								"repository": "https://r.googlesource.com/repo",
								"branch": "master",
								"revision": "deadbeef",
								"extra_prop": "val"
							}`),
							Tags: []string{
								"buildset:commit/git/deadbeef",
								"buildset:commit/gitiles/r.googlesource.com/repo/+/deadbeef",
								"gitiles_ref:ignored",
								"gitiles_ref:master",
								"extra:tag",
							},
						},
					},
				},
			})

			// Reconstructed gitiles commit from properties.
			So(req.GitilesCommit, ShouldResembleProto, &bbpb.GitilesCommit{
				Host:    "r.googlesource.com",
				Project: "repo",
				Id:      "deadbeef",
				Ref:     "refs/heads/master",
			})

			// Properties are sanitized.
			So(structKeys(req.Properties), ShouldResemble, []string{
				"$recipe_engine/scheduler",
				"extra_prop",
			})

			// Tags are sanitized too.
			So(req.Tags, ShouldResembleProto, []*bbpb.StringPair{
				{Key: "scheduler_invocation_id", Value: "1"},
				{Key: "scheduler_job_id", Value: "some-project/some-job"},
				{Key: "user_agent", Value: "app"},
				{Key: "a", Value: "from-task-def"},
				{Key: "b", Value: "from-task-def"},
				{Key: "extra", Value: "tag"},
			})
		})

		Convey("Branch is optional when reconstructing", func() {
			req := schedule([]*internal.Trigger{
				{
					Id: "1",
					Payload: &internal.Trigger_Buildbucket{
						Buildbucket: &api.BuildbucketTrigger{
							Properties: structFromJSON(`{
								"repository": "https://r.googlesource.com/repo",
								"revision": "deadbeef"
							}`),
							Tags: []string{
								"buildset:commit/gitiles/r.googlesource.com/repo/+/deadbeef",
							},
						},
					},
				},
			})
			So(req.GitilesCommit, ShouldResembleProto, &bbpb.GitilesCommit{
				Host:    "r.googlesource.com",
				Project: "repo",
				Id:      "deadbeef",
			})
			So(countTags(req.Tags, "buildset"), ShouldEqual, 0)
			So(countTags(req.Tags, "gitiles_ref"), ShouldEqual, 0)
		})

		Convey("Properties are ignored if buildset tag is missing", func() {
			req := schedule([]*internal.Trigger{
				{
					Id: "1",
					Payload: &internal.Trigger_Buildbucket{
						Buildbucket: &api.BuildbucketTrigger{
							Properties: structFromJSON(`{
								"repository": "https://r.googlesource.com/repo",
								"branch": "main",
								"revision": "deadbeef"
							}`),
							Tags: []string{
								"gitiles_ref:ignored",
							},
						},
					},
				},
			})
			So(req.GitilesCommit, ShouldBeNil)
			So(structKeys(req.Properties), ShouldResemble, []string{
				"$recipe_engine/scheduler",
			})
			So(countTags(req.Tags, "buildset"), ShouldEqual, 0)
			So(countTags(req.Tags, "gitiles_ref"), ShouldEqual, 0)
		})

		Convey("Tags are authoritative over properties", func() {
			req := schedule([]*internal.Trigger{
				{
					Id: "1",
					Payload: &internal.Trigger_Buildbucket{
						Buildbucket: &api.BuildbucketTrigger{
							Properties: structFromJSON(`{
								"repository": "https://prop.googlesource.com/repo-prop",
								"branch": "main-prop",
								"revision": "aaaa"
							}`),
							Tags: []string{
								"buildset:commit/gitiles/tag.googlesource.com/repo-tag/+/bbbb",
								"gitiles_ref:main-tag",
							},
						},
					},
				},
			})
			So(req.GitilesCommit, ShouldResembleProto, &bbpb.GitilesCommit{
				Host:    "tag.googlesource.com",
				Project: "repo-tag",
				Id:      "bbbb",
				Ref:     "refs/heads/main-tag",
			})
			So(structKeys(req.Properties), ShouldResemble, []string{
				"$recipe_engine/scheduler",
			})
			So(countTags(req.Tags, "buildset"), ShouldEqual, 0)
			So(countTags(req.Tags, "gitiles_ref"), ShouldEqual, 0)
		})
	})
}

func TestPassedTriggers(t *testing.T) {
	t.Parallel()

	Convey(fmt.Sprintf("Passed to buildbucket triggers are capped at %d", maxTriggersAsSchedulerProperty), t, func(ctx C) {
		c := memory.Use(context.Background())
		ctl := fakeController("doesn't matter")
		triggers := make([]*internal.Trigger, 0, maxTriggersAsSchedulerProperty+10)
		add := func(i int) {
			triggers = append(triggers, &internal.Trigger{
				Id: fmt.Sprintf("id=%d", i),
				Payload: &internal.Trigger_Gitiles{
					Gitiles: &api.GitilesTrigger{
						Repo:     "https://r.googlesource.com/repo",
						Ref:      "refs/heads/master",
						Revision: fmt.Sprintf("sha1=%d", i),
					},
				},
			})
		}
		for i := 0; i < maxTriggersAsSchedulerProperty; i++ {
			add(i)
		}

		propertiesString := func() string {
			ctl.Req = task.Request{IncomingTriggers: triggers}
			v, err := schedulerProperty(c, ctl)
			So(err, ShouldBeNil)
			return v.String()
		}

		s := propertiesString()
		So(s, ShouldContainSubstring, "sha1=0")
		So(s, ShouldContainSubstring, fmt.Sprintf("sha1=%d", maxTriggersAsSchedulerProperty-1))

		add(maxTriggersAsSchedulerProperty)
		s = propertiesString()
		So(s, ShouldContainSubstring, fmt.Sprintf("sha1=%d", maxTriggersAsSchedulerProperty))
		So(s, ShouldNotContainSubstring, "sha1=0")
	})
}

func TestExamineNotification(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		c := memory.Use(context.Background())
		mgr := TaskManager{}

		Convey("v1", func() {
			tok := mgr.ExamineNotification(c, &pubsub.PubsubMessage{
				Attributes: map[string]string{"auth_token": "blah"},
			})
			So(tok, ShouldEqual, "blah")
		})

		Convey("v2", func() {
			call := func(data string) string {
				return mgr.ExamineNotification(c, &pubsub.PubsubMessage{
					Data: data,
				})
			}
			So(call(base64.StdEncoding.EncodeToString([]byte(`{"user_data": "blah"}`))), ShouldEqual, "blah")
			So(call(base64.StdEncoding.EncodeToString([]byte(`not json`))), ShouldEqual, "")
			So(call("not base64"), ShouldEqual, "")
		})
	})
}

func structFromJSON(json string) *structpb.Struct {
	r := strings.NewReader(json)
	s := &structpb.Struct{}
	if err := (&jsonpb.Unmarshaler{}).Unmarshal(r, s); err != nil {
		panic(err)
	}
	return s
}

func structKeys(s *structpb.Struct) []string {
	keys := make([]string, 0, len(s.Fields))
	for k := range s.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func countTags(tags []*bbpb.StringPair, key string) (count int) {
	for _, tag := range tags {
		if tag.Key == key {
			count++
		}
	}
	return
}
