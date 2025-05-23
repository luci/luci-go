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
	"google.golang.org/api/option"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"
)

func TestCreate(t *testing.T) {
	t.Parallel()

	ftt.Run("createInstance", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		assert.Loosely(t, err, should.BeNil)
		c, _ := tsmon.WithDummyInMemory(memory.Use(context.Background()))
		c = withCompute(withDispatcher(c, dsp), ComputeService{Stable: gce})
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()
		s := tsmon.Store(c)

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := createInstance(c, nil)
				assert.Loosely(t, err, should.ErrLike("unexpected payload"))
			})

			t.Run("empty", func(t *ftt.Test) {
				err := createInstance(c, &tasks.CreateInstance{})
				assert.Loosely(t, err, should.ErrLike("ID is required"))
			})

			t.Run("missing", func(t *ftt.Test) {
				err := createInstance(c, &tasks.CreateInstance{
					Id: "id",
				})
				assert.Loosely(t, err, should.ErrLike("failed to fetch VM"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("exists", func(t *ftt.Test) {
				datastore.Put(c, &model.VM{
					ID:       "id",
					Hostname: "name",
					URL:      "url",
				})
				err := createInstance(c, &tasks.CreateInstance{
					Id: "id",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("drained", func(t *ftt.Test) {
				rt.Handler = func(req any) (int, any) {
					inst, ok := req.(*compute.Instance)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, inst.Name, should.Equal("name"))
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
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("error", func(t *ftt.Test) {
				t.Run("http", func(t *ftt.Test) {
					t.Run("transient", func(t *ftt.Test) {
						rt.Handler = func(req any) (int, any) {
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
						assert.Loosely(t, err, should.ErrLike("transiently failed to create instance"))
						v := &model.VM{
							ID: "id",
						}
						assert.Loosely(t, datastore.Get(c, v), should.BeNil)
						assert.Loosely(t, v.Hostname, should.Equal("name"))
					})

					t.Run("permanent", func(t *ftt.Test) {
						rt.Handler = func(req any) (int, any) {
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
						assert.Loosely(t, err, should.ErrLike("failed to create instance"))
						v := &model.VM{
							ID: "id",
						}
						assert.Loosely(t, datastore.Get(c, v), should.Equal(datastore.ErrNoSuchEntity))
					})
				})

				t.Run("operation", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
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
					assert.Loosely(t, err, should.ErrLike("failed to create instance"))
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.Equal(datastore.ErrNoSuchEntity))
				})
			})

			t.Run("created", func(t *ftt.Test) {
				confFields := []any{"", "", "", "", "name"}
				assert.Loosely(t, s.Get(c, metrics.CreatedInstanceChecked, confFields), should.BeNil)
				t.Run("pending", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
						inst, ok := req.(*compute.Instance)
						assert.Loosely(t, ok, should.BeTrue)
						assert.Loosely(t, inst.Name, should.Equal("name"))
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
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, v.Created, should.BeZero)
					assert.Loosely(t, v.URL, should.BeEmpty)
				})

				t.Run("done", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
						switch rt.Type {
						case reflect.TypeOf(compute.Instance{}):
							// First call, to create the instance.
							inst, ok := req.(*compute.Instance)
							assert.Loosely(t, ok, should.BeTrue)
							assert.Loosely(t, inst.Name, should.Equal("name"))
							rt.Type = reflect.TypeOf(map[string]string{})
							return http.StatusOK, &compute.Operation{
								EndTime:    "2018-12-14T15:07:48.200-08:00",
								Status:     "DONE",
								TargetLink: "url",
							}
						default:
							// Second call, to check the reason for the conflict.
							// This request should have no body.
							assert.Loosely(t, *(req.(*map[string]string)), should.HaveLength(0))
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
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					assert.Loosely(t, datastore.Get(c, v), should.BeNil)
					assert.Loosely(t, v.Created, should.NotEqual(0))
					assert.Loosely(t, v.NetworkInterfaces, should.Match([]model.NetworkInterface{
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
					}))
					assert.Loosely(t, v.URL, should.Equal("url"))
					assert.Loosely(t, s.Get(c, metrics.CreatedInstanceChecked, confFields), should.Equal(1))
				})
			})
		})
	})
}

func TestDestroyInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("destroyInstance", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		assert.Loosely(t, err, should.BeNil)
		c := withCompute(withDispatcher(memory.Use(context.Background()), dsp), ComputeService{Stable: gce})
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := destroyInstance(c, nil)
				assert.Loosely(t, err, should.ErrLike("unexpected payload"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("empty", func(t *ftt.Test) {
				err := destroyInstance(c, &tasks.DestroyInstance{})
				assert.Loosely(t, err, should.ErrLike("ID is required"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("url", func(t *ftt.Test) {
				err := destroyInstance(c, &tasks.DestroyInstance{
					Id: "id",
				})
				assert.Loosely(t, err, should.ErrLike("URL is required"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("missing", func(t *ftt.Test) {
				err := destroyInstance(c, &tasks.DestroyInstance{
					Id:  "id",
					Url: "url",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("replaced", func(t *ftt.Test) {
				datastore.Put(c, &model.VM{
					ID:  "id",
					URL: "new",
				})
				err := destroyInstance(c, &tasks.DestroyInstance{
					Id:  "id",
					Url: "old",
				})
				assert.Loosely(t, err, should.BeNil)
				v := &model.VM{
					ID: "id",
				}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.URL, should.Equal("new"))
			})

			t.Run("error", func(t *ftt.Test) {
				t.Run("http", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
						return http.StatusInternalServerError, nil
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						URL:      "url",
						Hostname: "name",
					})
					err := destroyInstance(c, &tasks.DestroyInstance{
						Id:  "id",
						Url: "url",
					})
					assert.Loosely(t, err, should.ErrLike("destroy instance \"name\": googleapi: got HTTP response code 500 with body: null"))
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					assert.Loosely(t, v.URL, should.Equal("url"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("operation", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
						return http.StatusOK, &compute.Operation{
							Error: &compute.OperationError{
								Errors: []*compute.OperationErrorErrors{
									{},
								},
							},
						}
					}
					datastore.Put(c, &model.VM{
						ID:       "id",
						URL:      "url",
						Hostname: "name",
					})
					err := destroyInstance(c, &tasks.DestroyInstance{
						Id:  "id",
						Url: "url",
					})
					assert.Loosely(t, err, should.ErrLike("Destroy instance \"name\": failed to destroy"))
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					assert.Loosely(t, v.URL, should.Equal("url"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})
			})

			t.Run("destroys", func(t *ftt.Test) {
				t.Run("pending", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
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
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					assert.Loosely(t, v.Created, should.Equal(1))
					assert.Loosely(t, v.Hostname, should.Equal("name"))
					assert.Loosely(t, v.URL, should.Equal("url"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
				})

				t.Run("done", func(t *ftt.Test) {
					rt.Handler = func(req any) (int, any) {
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
					assert.Loosely(t, err, should.BeNil)
					v := &model.VM{
						ID: "id",
					}
					datastore.Get(c, v)
					assert.Loosely(t, v.Created, should.Equal(1))
					assert.Loosely(t, v.Hostname, should.Equal("name"))
					assert.Loosely(t, v.URL, should.Equal("url"))
					assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
				})
			})
		})
	})
}

func TestAuditInstanceInZone(t *testing.T) {
	t.Parallel()

	ftt.Run("auditInstanceInZone", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		c := context.Background()
		gce, err := compute.NewService(c, option.WithHTTPClient(&http.Client{Transport: rt}))
		assert.Loosely(t, err, should.BeNil)
		c = withCompute(withDispatcher(memory.Use(c), dsp), ComputeService{Stable: gce})
		datastore.GetTestable(c).Consistent(true)
		tqt := tqtesting.GetTestable(c, dsp)
		tqt.CreateQueues()

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("nil", func(t *ftt.Test) {
				err := auditInstanceInZone(c, nil)
				assert.Loosely(t, err, should.ErrLike("Unexpected payload"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("empty", func(t *ftt.Test) {
				err := auditInstanceInZone(c, &tasks.AuditProject{})
				assert.Loosely(t, err, should.ErrLike("Project is required"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("empty region", func(t *ftt.Test) {
				err := auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
				})
				assert.Loosely(t, err, should.ErrLike("Zone is required"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("VM entry exists", func(t *ftt.Test) {
				count := 0
				// The first request must be to the List API. Should not
				// do any consequent requests
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.InstanceList{
							Items: []*compute.Instance{{
								Name: "double-11-puts",
							}},
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.VM{
					ID:       "double-11",
					Hostname: "double-11-puts",
					Prefix:   "double",
				})
				assert.Loosely(t, err, should.BeNil)
				err = auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(1))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("VM entry exists double, page token", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.InstanceList{
							Items: []*compute.Instance{{
								Name: "double-11-puts",
							}, {
								Name: "thes-1-puts",
							}},
							NextPageToken: "next-page",
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.VM{
					ID:       "double-11",
					Hostname: "double-11-puts",
					Prefix:   "double",
				})
				assert.Loosely(t, err, should.BeNil)
				err = datastore.Put(c, &model.VM{
					ID:       "thes-1",
					Hostname: "thes-1-puts",
					Prefix:   "thes",
				})
				assert.Loosely(t, err, should.BeNil)
				err = auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(1))
				// The next token should schedule a job
				assert.Loosely(t, tqt.GetScheduledTasks(), should.HaveLength(1))
			})

			t.Run("VM leaked (single)", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.InstanceList{
							Items: []*compute.Instance{{
								Name: "double-11-acrd",
							}},
						}
					case 1:
						count += 1
						return http.StatusOK, &compute.Operation{}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.VM{
					ID:       "double-11",
					Hostname: "double-11-puts",
					Prefix:   "double",
				})
				assert.Loosely(t, err, should.BeNil)
				err = auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(2))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("VM leaked (double)", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.InstanceList{
							Items: []*compute.Instance{{
								Name: "double-12-acrd",
							}, {
								Name: "thes-1-acrd",
							}},
						}
					case 1:
						count += 1
						return http.StatusOK, &compute.Operation{}
					case 2:
						count += 1
						return http.StatusOK, &compute.Operation{}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.VM{
					ID:       "double-11",
					Hostname: "double-11-puts",
					Prefix:   "double",
				})
				assert.Loosely(t, err, should.BeNil)
				err = datastore.Put(c, &model.VM{
					ID:       "thes-1",
					Hostname: "thes-1-puts",
					Prefix:   "thes",
				})
				assert.Loosely(t, err, should.BeNil)
				err = auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, count, should.Equal(3))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})

			t.Run("VM leaked (mix)", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.InstanceList{
							Items: []*compute.Instance{{
								Name: "double-12-acrd",
							}, {
								Name: "thes-1-puts",
							}},
						}
					case 1:
						count += 1
						return http.StatusOK, &compute.Operation{
							Status: "Done",
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.VM{
					ID:       "double-11",
					Hostname: "double-11-puts",
					Prefix:   "double",
				})
				assert.Loosely(t, err, should.BeNil)
				err = auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, count, should.Equal(2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})
		})

		t.Run("error", func(t *ftt.Test) {
			t.Run("list failure", func(t *ftt.Test) {
				rt.Handler = func(req any) (int, any) {
					return http.StatusInternalServerError, nil
				}
				err := auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, err, should.ErrLike("failed to list"))
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})
			t.Run("VM delete failure", func(t *ftt.Test) {
				count := 0
				rt.Handler = func(req any) (int, any) {
					switch count {
					case 0:
						count += 1
						return http.StatusOK, &compute.InstanceList{
							Items: []*compute.Instance{{
								Name: "double-12-acrd",
							}},
						}
					case 1:
						count += 1
						return http.StatusOK, &compute.Operation{
							Error: &compute.OperationError{
								Errors: []*compute.OperationErrorErrors{
									{},
								},
							},
						}
					default:
						count += 1
						return http.StatusInternalServerError, nil
					}
				}
				err := datastore.Put(c, &model.VM{
					ID:       "double-11",
					Hostname: "double-11-puts",
					Prefix:   "double",
				})
				assert.Loosely(t, err, should.BeNil)
				err = auditInstanceInZone(c, &tasks.AuditProject{
					Project: "libreboot",
					Zone:    "us-mex-1",
				})
				assert.Loosely(t, count, should.Equal(2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tqt.GetScheduledTasks(), should.BeEmpty)
			})
		})
	})
}

func TestIsLeakHuerestic(t *testing.T) {
	t.Parallel()

	ftt.Run("isLeakHuerestic", t, func(t *ftt.Test) {
		c := context.Background()
		c = memory.Use(c)
		datastore.GetTestable(c).Consistent(true)

		err := datastore.Put(c, &model.VM{
			ID:       "host-vm-test-time-10",
			Hostname: "host-vm-test-time-10-xyz3",
			Prefix:   "host-vm-test-time",
		})
		assert.Loosely(t, err, should.BeNil)
		err = datastore.Put(c, &model.VM{
			ID:       "host-vm-test-time-11",
			Hostname: "host-vm-test-time-11-xwz2",
			Prefix:   "host-vm-test-time",
		})
		assert.Loosely(t, err, should.BeNil)
		t.Run("positive results", func(t *ftt.Test) {
			t.Run("valid leak with replacement", func(t *ftt.Test) {
				// Leak replaced by host-vm-test-time-10-xyz3
				leak := isLeakHuerestic(c, "host-vm-test-time-10-ijk1", "project", "us-numba-1")
				assert.Loosely(t, leak, should.BeTrue)
			})
			t.Run("valid leak resized pool", func(t *ftt.Test) {
				// Leak and pool resized
				leak := isLeakHuerestic(c, "host-vm-test-time-12-abc2", "project", "us-numba-1")
				assert.Loosely(t, leak, should.BeTrue)
			})
		})
		t.Run("negative results", func(t *ftt.Test) {
			t.Run("non gce-provider instance", func(t *ftt.Test) {
				// gce-provider didn't create this instance
				leak := isLeakHuerestic(c, "sha512-collision-detect", "project", "us-numba-1")
				assert.Loosely(t, leak, should.BeFalse)
			})
			t.Run("instance without a current config", func(t *ftt.Test) {
				// Config is deleted for this prefix
				leak := isLeakHuerestic(c, "dut-12-abc2", "project", "us-numba-1")
				assert.Loosely(t, leak, should.BeFalse)
			})
		})
	})
}
