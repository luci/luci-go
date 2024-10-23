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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestHandleViewBuild(t *testing.T) {
	t.Parallel()

	ftt.Run("handleViewBuild", t, func(t *ftt.Test) {
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
		assert.Loosely(t, datastore.Put(ctx, build, bucket), should.BeNil)

		t.Run("invalid build id", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "foo"},
			}

			handleViewBuild(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring("invalid build id"))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
			}

			handleViewBuild(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`resource not found or "user:user@example.com" does not have permission to view it`))
		})

		t.Run("build not exist", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "9"},
			}

			handleViewBuild(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`resource not found or "user:user@example.com" does not have permission to view it`))
		})

		t.Run("anonymous", func(t *ftt.Test) {
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
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
			assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("http://fake.example.com/login?dest=%2Fbuild%2F123"))
		})

		t.Run("ok", func(t *ftt.Test) {
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
			assert.Loosely(t, config.SetTestSettingsCfg(ctx, settingsCfg), should.BeNil)

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
			}

			handleViewBuild(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
			assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("https://milo.com/b/123"))
		})

		t.Run("build with view_url", func(t *ftt.Test) {
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
			assert.Loosely(t, config.SetTestSettingsCfg(ctx, settingsCfg), should.BeNil)
			url := "https://another.com"
			b := &model.Build{
				ID: 300,
				Proto: &pb.Build{
					Id: 300,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					ViewUrl: url,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "300"},
			}
			handleViewBuild(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
			assert.Loosely(t, rsp.Header().Get("Location"), should.Equal(url))
		})

		t.Run("RedirectToTaskPage", func(t *ftt.Test) {
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
			assert.Loosely(t, config.SetTestSettingsCfg(ctx, settingsCfg), should.BeNil)
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

			t.Run("task.Link is populated", func(t *ftt.Test) {
				bInfra.Proto.Backend.Task.Link = "https://fake-task-page-link"
				assert.Loosely(t, datastore.Put(ctx, bInfra), should.BeNil)
				rctx.Request = (&http.Request{}).WithContext(ctx)
				rctx.Params = httprouter.Params{
					{Key: "BuildID", Value: "123"},
				}

				handleViewBuild(rctx)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
				assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("https://fake-task-page-link"))
			})

			t.Run("task.Link is not populated", func(t *ftt.Test) {
				bInfra.Proto.Backend.Task.Link = ""
				assert.Loosely(t, datastore.Put(ctx, bInfra), should.BeNil)
				rctx.Request = (&http.Request{}).WithContext(ctx)
				rctx.Params = httprouter.Params{
					{Key: "BuildID", Value: "123"},
				}

				handleViewBuild(rctx)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
				assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("https://milo.com/b/123"))
			})
		})
		t.Run("builder redirect", func(t *ftt.Test) {
			rsp := httptest.NewRecorder()
			rctx := &router.Context{
				Writer: rsp,
			}
			ctx := memory.Use(context.Background())
			settingsCfg := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					MiloHostname: "milo.com",
				},
			}
			assert.Loosely(t, config.SetTestSettingsCfg(ctx, settingsCfg), should.BeNil)
			rctx.Request = (&http.Request{}).WithContext(ctx)

			t.Run("invalid parameters", func(t *ftt.Test) {
				t.Run("project", func(t *ftt.Test) {
					rctx.Params = httprouter.Params{}
					handleViewBuilder(rctx)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
				})
				t.Run("bucket", func(t *ftt.Test) {
					rctx.Params = httprouter.Params{
						{Key: "Project", Value: "project"},
					}
					handleViewBuilder(rctx)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
				})
				t.Run("builder", func(t *ftt.Test) {
					rctx.Params = httprouter.Params{
						{Key: "Project", Value: "project"},
						{Key: "Bucket", Value: "bucket"},
					}
					handleViewBuilder(rctx)
					assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				rctx.Params = httprouter.Params{
					{Key: "Project", Value: "project"},
					{Key: "Bucket", Value: "bucket"},
					{Key: "Builder", Value: "builder"},
				}
				handleViewBuilder(rctx)
				assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
				assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("https://milo.com/ui/p/project/builders/bucket/builder"))
			})
		})
	})
}

func TestHandleViewLog(t *testing.T) {
	t.Parallel()

	ftt.Run("handleViewLog", t, func(t *ftt.Test) {
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
		assert.Loosely(t, bs.FromProto([]*pb.Step{
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
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, build, bucket, bs), should.BeNil)

		t.Run("invalid build id", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "foo"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring("invalid build id"))
		})

		t.Run("no access to build", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
			})

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`resource not found or "user:random@example.com" does not have permission to view it`))
		})

		t.Run("build not found", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "9"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`resource not found or "user:user@example.com" does not have permission to view it`))
		})

		t.Run("no steps", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}
			assert.Loosely(t, datastore.Delete(ctx, bs), should.BeNil)

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring("no steps found"))
		})

		t.Run("the requested step not found", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "second"},
			}

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`view url for log "stdout" in step "second" in build 123 not found`))
		})

		t.Run("the requested log not found", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{
				RawQuery: "log=stderr",
			}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`view url for log "stderr" in step "first" in build 123 not found`))
		})

		t.Run("ok", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Request.URL = &url.URL{}
			rctx.Params = httprouter.Params{
				{Key: "BuildID", Value: "123"},
				{Key: "StepName", Value: "first"},
			}

			handleViewLog(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
			assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("https://log.com/123/first"))
		})
	})
}

func TestHandleMain(t *testing.T) {
	t.Parallel()

	ftt.Run("handleMain", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		settingsCfg := &pb.SettingsCfg{
			Swarming: &pb.SwarmingSettings{
				MiloHostname: "milo.com",
			},
		}

		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}
		rctx.Request = (&http.Request{}).WithContext(ctx)
		rctx.Request.URL = &url.URL{
			Path: "/",
		}

		assert.Loosely(t, config.SetTestSettingsCfg(ctx, settingsCfg), should.BeNil)
		handleMain(rctx)
		assert.Loosely(t, rsp.Code, should.Equal(http.StatusFound))
		assert.Loosely(t, rsp.Header().Get("Location"), should.Equal("https://milo.com/ui/"))
	})
}
