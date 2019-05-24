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

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	rpc "go.chromium.org/luci/gce/appengine/rpc/memory"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	Convey("createInstance", t, func() {
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
				err := createInstance(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
			})

			Convey("empty", func() {
				err := createInstance(c, &tasks.CreateInstance{})
				So(err, ShouldErrLike, "ID is required")
			})

			Convey("missing", func() {
				err := createInstance(c, &tasks.CreateInstance{
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
				err := createInstance(c, &tasks.CreateInstance{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("drained", func() {
				rt.Handler = func(req interface{}) (int, interface{}) {
					inst, ok := req.(*compute.Instance)
					So(ok, ShouldBeTrue)
					So(inst.Name, ShouldEqual, "name")
					return http.StatusOK, &compute.Operation{}
				}
				rt.Type = reflect.TypeOf(compute.Instance{})
				datastore.Put(c, &model.VM{
					ID:       "id",
					Drained:  true,
					Hostname: "name",
				})
				err := createInstance(c, &tasks.CreateInstance{
					Id: "id",
				})
				So(err, ShouldBeNil)
			})

			Convey("error", func() {
				Convey("http", func() {
					Convey("transient", func() {
						rt.Handler = func(req interface{}) (int, interface{}) {
							return http.StatusInternalServerError, nil
						}
						rt.Type = reflect.TypeOf(compute.Instance{})
						datastore.Put(c, &model.VM{
							ID:       "id",
							Hostname: "name",
						})
						err := createInstance(c, &tasks.CreateInstance{
							Id: "id",
						})
						So(err, ShouldErrLike, "transiently failed to create instance")
						v := &model.VM{
							ID: "id",
						}
						So(datastore.Get(c, v), ShouldBeNil)
						So(v.Hostname, ShouldEqual, "name")
					})

					Convey("permanent", func() {
						rt.Handler = func(req interface{}) (int, interface{}) {
							return http.StatusConflict, nil
						}
						rt.Type = reflect.TypeOf(compute.Instance{})
						datastore.Put(c, &model.VM{
							ID:       "id",
							Hostname: "name",
						})
						err := createInstance(c, &tasks.CreateInstance{
							Id: "id",
						})
						So(err, ShouldErrLike, "failed to create instance")
						v := &model.VM{
							ID: "id",
						}
						So(datastore.Get(c, v), ShouldEqual, datastore.ErrNoSuchEntity)
					})
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
					err := createInstance(c, &tasks.CreateInstance{
						Id: "id",
					})
					So(err, ShouldErrLike, "failed to create instance")
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldEqual, datastore.ErrNoSuchEntity)
				})
			})

			Convey("created", func() {
				Convey("pending", func() {
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
					err := createInstance(c, &tasks.CreateInstance{
						Id: "id",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Created, ShouldEqual, 0)
					So(v.URL, ShouldBeEmpty)
				})

				Convey("done", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						switch rt.Type {
						case reflect.TypeOf(compute.Instance{}):
							// First call, to create the instance.
							inst, ok := req.(*compute.Instance)
							So(ok, ShouldBeTrue)
							So(inst.Name, ShouldEqual, "name")
							rt.Type = reflect.TypeOf(map[string]string{})
							return http.StatusOK, &compute.Operation{
								EndTime:    "2018-12-14T15:07:48.200-08:00",
								Status:     "DONE",
								TargetLink: "url",
							}
						default:
							// Second call, to check the reason for the conflict.
							// This request should have no body.
							So(*(req.(*map[string]string)), ShouldHaveLength, 0)
							return http.StatusOK, &compute.Instance{
								CreationTimestamp: "2018-12-14T15:07:48.200-08:00",
								NetworkInterfaces: []*compute.NetworkInterface{
									{
										NetworkIP: "0.0.0.1",
									},
									{
										AccessConfigs: []*compute.AccessConfig{
											{
												NatIP: "2.0.0.0",
											},
										},
										NetworkIP: "0.0.0.2",
									},
									{
										AccessConfigs: []*compute.AccessConfig{
											{
												NatIP: "3.0.0.0",
											},
											{
												NatIP: "3.0.0.1",
											},
										},
										NetworkIP: "0.0.0.3",
									},
								},
								SelfLink: "url",
							}
						}
					}
					rt.Type = reflect.TypeOf(compute.Instance{})
					datastore.Put(c, &model.VM{
						ID:       "id",
						Hostname: "name",
					})
					err := createInstance(c, &tasks.CreateInstance{
						Id: "id",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					So(datastore.Get(c, v), ShouldBeNil)
					So(v.Created, ShouldNotEqual, 0)
					So(v.NetworkInterfaces, ShouldResemble, []model.NetworkInterface{
						{
							InternalIP: "0.0.0.1",
						},
						{
							ExternalIP: "2.0.0.0",
							InternalIP: "0.0.0.2",
						},
						{
							ExternalIP: "3.0.0.0",
							InternalIP: "0.0.0.3",
						},
					})
					So(v.URL, ShouldEqual, "url")
				})
			})
		})
	})
}

func TestDestroyInstance(t *testing.T) {
	t.Parallel()

	Convey("destroyInstance", t, func() {
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
				err := destroyInstance(c, nil)
				So(err, ShouldErrLike, "unexpected payload")
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("empty", func() {
				err := destroyInstance(c, &tasks.DestroyInstance{})
				So(err, ShouldErrLike, "ID is required")
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("url", func() {
				err := destroyInstance(c, &tasks.DestroyInstance{
					Id: "id",
				})
				So(err, ShouldErrLike, "URL is required")
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})
		})

		Convey("valid", func() {
			Convey("missing", func() {
				err := destroyInstance(c, &tasks.DestroyInstance{
					Id:  "id",
					Url: "url",
				})
				So(err, ShouldBeNil)
				So(tqt.GetScheduledTasks(), ShouldBeEmpty)
			})

			Convey("replaced", func() {
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "new",
				})
				err := destroyInstance(c, &tasks.DestroyInstance{
					Id:  "id",
					Url: "old",
				})
				So(err, ShouldBeNil)
				v := &model.VM{
					ID: "id",
				}
				So(datastore.Get(c, v), ShouldBeNil)
				So(v.URL, ShouldEqual, "new")
			})

			Convey("error", func() {
				Convey("http", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						return http.StatusInternalServerError, nil
					}
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					err := destroyInstance(c, &tasks.DestroyInstance{
						Id:  "id",
						Url: "url",
					})
					So(err, ShouldErrLike, "failed to destroy instance")
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.URL, ShouldEqual, "url")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
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
					datastore.Put(c, &model.VM{
						ID:  "id",
						URL: "url",
					})
					err := destroyInstance(c, &tasks.DestroyInstance{
						Id:  "id",
						Url: "url",
					})
					So(err, ShouldErrLike, "failed to destroy instance")
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.URL, ShouldEqual, "url")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})
			})

			Convey("destroys", func() {
				Convey("pending", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						return http.StatusOK, &compute.Operation{}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Created:  1,
						Hostname: "name",
						URL:      "url",
					})
					err := destroyInstance(c, &tasks.DestroyInstance{
						Id:  "id",
						Url: "url",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Created, ShouldEqual, 1)
					So(v.Hostname, ShouldEqual, "name")
					So(v.URL, ShouldEqual, "url")
					So(tqt.GetScheduledTasks(), ShouldBeEmpty)
				})

				Convey("done", func() {
					rt.Handler = func(req interface{}) (int, interface{}) {
						return http.StatusOK, &compute.Operation{
							Status:     "DONE",
							TargetLink: "url",
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						Created:  1,
						Hostname: "name",
						URL:      "url",
					})
					err := destroyInstance(c, &tasks.DestroyInstance{
						Id:  "id",
						Url: "url",
					})
					So(err, ShouldBeNil)
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					So(v.Created, ShouldEqual, 1)
					So(v.Hostname, ShouldEqual, "name")
					So(v.URL, ShouldEqual, "url")
					So(tqt.GetScheduledTasks(), ShouldHaveLength, 1)
				})
			})
		})
	})
}
