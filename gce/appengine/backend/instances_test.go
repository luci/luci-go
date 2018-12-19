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

package backend

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/compute/v1"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	Convey("create", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withCompute(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), gce)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("invalid", func() {
			Convey("nil", func() {
				err := create(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
			})

			Convey("empty", func() {
				err := create(c, &tasks.Create{})
				So(err, ShouldErrLike, "ID is required")
			})

			Convey("missing", func() {
				err := create(c, &tasks.Create{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch VM")
			})
		})

		Convey("valid", func() {
			Convey("exists", func() {
				datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
					URL:      "url",
				})
				err := create(c, &tasks.Create{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("error", func() {
				Convey("http", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						return http.StatusInternalServerError, nil
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to create instance")
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Hostname, ShouldBeEmpty)
				})

				Convey("operation", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						return http.StatusOK, &compute.Operation{
							Error: &compute.OperationError{
								Errors: []*compute.OperationErrorErrors{
									{},
								},
							},
						}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to create instance")
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Hostname, ShouldBeEmpty)
				})
			})

			Convey("conflict", func() {
				Convey("zone", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						switch rt.Type {
						case reflect.TypeOf(compute.Instance{}):
							// First call, to create the instance.
							rt.Type = reflect.TypeOf(map[string]string{})
							return http.StatusConflict, nil
						default:
							// Second call, to check the reason for the conflict.
							// This request should have no body.
							So(*(req.(*map[string]string)), ShouldHaveLength, 0)
							return http.StatusNotFound, nil
						}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldErrLike, "instance exists in another zone")
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Hostname, ShouldBeEmpty)
					So(v.URL, ShouldBeEmpty)
				})

				Convey("exists", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						switch rt.Type {
						case reflect.TypeOf(compute.Instance{}):
							// First call, to create the instance.
							rt.Type = reflect.TypeOf(map[string]string{})
							return http.StatusConflict, nil
						default:
							// Second call, to check the reason for the conflict.
							// This request should have no body.
							So(*(req.(*map[string]string)), ShouldHaveLength, 0)
							return http.StatusOK, &compute.Instance{
								CreationTimestamp: "2018-12-14T15:07:48.200-08:00",
								SelfLink:          "url",
							}
						}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Deadline, ShouldNotEqual, 0)
					So(v.URL, ShouldEqual, "url")
				})
			})

			Convey("creates", func() {
				Convey("names", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						inst, ok := req.(*compute.Instance)
						So(ok, ShouldBeTrue)
						So(inst.Name, ShouldNotBeEmpty)
						return http.StatusOK, &compute.Operation{}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID: "id",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldBeNil)
				})

				Convey("named", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						inst, ok := req.(*compute.Instance)
						So(ok, ShouldBeTrue)
						So(inst.Name, ShouldEqual, "name")
						return http.StatusOK, &compute.Operation{}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldBeNil)
				})

				Convey("done", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						inst, ok := req.(*compute.Instance)
						So(ok, ShouldBeTrue)
						So(inst.Name, ShouldEqual, "name")
						return http.StatusOK, &compute.Operation{
							EndTime:    "2018-12-14T15:07:48.200-08:00",
							Status:     "DONE",
							TargetLink: "url",
						}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := create(c, &tasks.Create{
						Id: "id",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Deadline, ShouldNotEqual, 0)
					So(v.URL, ShouldEqual, "url")
				})
			})
		})
	})
}

func TestManage(t *testing.T) {
	t.Parallel()

	Convey("manage", t, func() {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		srv := &rpc.Config{}
		rt := &roundtripper.JSONRoundTripper{}
		swr, err := swarming.New(&http.Client{Transport: rt})
		So(err, ShouldBeNil)
		c := withSwarming(withConfig(withDispatcher(memory.Use(context.Background()), dsp), srv), swr)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		Convey("invalid", func() {
			Convey("nil", func() {
				err := manage(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
			})

			Convey("empty", func() {
				err := manage(c, &tasks.Manage{})
				So(err, ShouldErrLike, "ID is required")
			})

			Convey("missing", func() {
				err := manage(c, &tasks.Manage{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch VM")
			})
		})

		Convey("valid", func() {
			Convey("error", func() {
				rt.Handler = func(_ interface{}) (int, interface{}) {
					return http.StatusConflict, nil
				}
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				err := manage(c, &tasks.Manage{
					Id: "id",
				})
				So(err, ShouldErrLike, "failed to fetch bot")
			})

			Convey("missing", func() {
				rt.Handler = func(_ interface{}) (int, interface{}) {
					return http.StatusNotFound, nil
				}
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				err := manage(c, &tasks.Manage{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("found", func() {
				rt.Handler = func(_ interface{}) (int, interface{}) {
					return http.StatusOK, &swarming.SwarmingRpcsBotInfo{
						BotId: "id",
					}
				}
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "url",
				})
				err := manage(c, &tasks.Manage{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})
		})
	})
}
