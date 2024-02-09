// Copyright 2024 The LUCI Authors.
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

package redirect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleViewBuild(t *testing.T) {
	t.Parallel()

	Convey("handleViewBuild", t, func() {
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}

		userID := identity.Identity("user:user@example.com")
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
		})

		build := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
			BackendTarget: "foo",
		}
		bucket := &model.Bucket{ID: "bucket", Parent: model.ProjectKey(ctx, "project")}
		So(datastore.Put(ctx, build, bucket), ShouldBeNil)

		Convey("invalid build id", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "foo"},
			}

			handleViewBuild(rctx)
			So(rsp.Code, ShouldEqual, http.StatusBadRequest)
			So(rsp.Body.String(), ShouldContainSubstring, "invalid build id")
		})

		Convey("permission denied", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
			}

			handleViewBuild(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `resource not found or "user:user@example.com" does not have permission to view it`)
		})

		Convey("build not exist", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "9"},
			}

			handleViewBuild(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `resource not found or "user:user@example.com" does not have permission to view it`)
		})

		Convey("anonymous", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.AnonymousIdentity,
			})
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{
				Path: "/build/123",
			}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
			}

			handleViewBuild(rctx)
			So(rsp.Code, ShouldEqual, http.StatusFound)
			So(rsp.Header().Get("Location"), ShouldEqual, "http://fake.example.com/login?dest=%2Fbuild%2F123")
		})

		Convey("ok", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				),
			})

			settingsCfg := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					MiloHostname: "milo.com",
				},
			}
			So(config.SetTestSettingsCfg(ctx, settingsCfg), ShouldBeNil)

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
			}

			handleViewBuild(rctx)
			So(rsp.Code, ShouldEqual, http.StatusFound)
			So(rsp.Header().Get("Location"), ShouldEqual, "https://milo.com/b/123")
		})

		Convey("RedirectToTaskPage", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: userID,
				FakeDB: authtest.NewFakeDB(
					authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				),
			})

			settingsCfg := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					MiloHostname: "milo.com",
				},
				Backends: []*pb.BackendSetting{
					{
						Target: "foo",
						Mode: &pb.BackendSetting_FullMode_{
							FullMode: &pb.BackendSetting_FullMode{
								RedirectToTaskPage: true,
							},
						},
					},
				},
			}
			So(config.SetTestSettingsCfg(ctx, settingsCfg), ShouldBeNil)
			bInfra := &model.BuildInfra{
				Build: datastore.KeyForObj(ctx, build),
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "task1",
								Target: "foo",
							},
						},
					},
				},
			}

			Convey("task.Link is populated", func() {
				bInfra.Proto.Backend.Task.Link = "https://fake-task-page-link"
				So(datastore.Put(ctx, bInfra), ShouldBeNil)
				rctx.Request = (&http.Request{}).WithContext(ctx)
				rctx.Params = httprouter.Params{
					{Key: "BuildID", Value: "123"},
				}

				handleViewBuild(rctx)
				So(rsp.Code, ShouldEqual, http.StatusFound)
				So(rsp.Header().Get("Location"), ShouldEqual, "https://fake-task-page-link")
			})

			Convey("task.Link is not populated", func() {
				bInfra.Proto.Backend.Task.Link = ""
				So(datastore.Put(ctx, bInfra), ShouldBeNil)
				rctx.Request = (&http.Request{}).WithContext(ctx)
				rctx.Params = httprouter.Params{
					{Key: "BuildID", Value: "123"},
				}

				handleViewBuild(rctx)
				So(rsp.Code, ShouldEqual, http.StatusFound)
				So(rsp.Header().Get("Location"), ShouldEqual, "https://milo.com/b/123")
			})
		})
	})
}

func TestHandleViewLog(t *testing.T) {
	t.Parallel()

	Convey("handleViewLog", t, func() {
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}

		userID := identity.Identity("user:user@example.com")
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
			),
		})

		build := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}
		bucket := &model.Bucket{ID: "bucket", Parent: model.ProjectKey(ctx, "project")}
		bs := &model.BuildSteps{
			Build: datastore.KeyForObj(ctx, build),
		}
		So(bs.FromProto([]*pb.Step{
			{
				Name:            "first",
				SummaryMarkdown: "summary",
				Logs: []*pb.Log{{
					Name:    "stdout",
					Url:     "url",
					ViewUrl: "https://log.com/123/first",
				},
				},
			},
		}), ShouldBeNil)
		So(datastore.Put(ctx, build, bucket, bs), ShouldBeNil)

		Convey("invalid build id", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "foo"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusBadRequest)
			So(rsp.Body.String(), ShouldContainSubstring, "invalid build id")
		})

		Convey("no access to build", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
			})

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `resource not found or "user:random@example.com" does not have permission to view it`)
		})

		Convey("build not found", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "9"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `resource not found or "user:user@example.com" does not have permission to view it`)
		})

		Convey("no steps", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}
			So(datastore.Delete(ctx, bs), ShouldBeNil)

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, "no steps found")
		})

		Convey("the requested step not found", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "second"},
			}

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `view url for log "stdout" in step "second" in build 123 not found`)
		})

		Convey("the requested log not found", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{
				RawQuery: "log=stderr",
			}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `view url for log "stderr" in step "first" in build 123 not found`)
		})

		Convey("ok", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			So(rsp.Code, ShouldEqual, http.StatusFound)
			So(rsp.Header().Get("Location"), ShouldEqual, "https://log.com/123/first")
		})
	})
}
