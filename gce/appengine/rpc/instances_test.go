// Copyright 2019 The LUCI Authors.
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

package rpc

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/instances/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

func TestInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("Instances", t, func(t *ftt.Test) {
		srv := &Instances{}
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		t.Run("Delete", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					_, err := srv.Delete(c, nil)
					assert.Loosely(t, err, should.ErrLike("ID or hostname is required"))
				})

				t.Run("empty", func(t *ftt.Test) {
					req := &instances.DeleteRequest{}
					_, err := srv.Delete(c, req)
					assert.Loosely(t, err, should.ErrLike("ID or hostname is required"))
				})

				t.Run("both", func(t *ftt.Test) {
					req := &instances.DeleteRequest{
						Id:       "id",
						Hostname: "hostname",
					}
					_, err := srv.Delete(c, req)
					assert.Loosely(t, err, should.ErrLike("exactly one of ID or hostname is required"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				vm := &model.VM{
					ID:       "id",
					Hostname: "hostname",
				}

				t.Run("id", func(t *ftt.Test) {
					req := &instances.DeleteRequest{
						Id: "id",
					}

					t.Run("deletes", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, vm), should.BeNil)
						rsp, err := srv.Delete(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp, should.Resemble(&emptypb.Empty{}))
						assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
						assert.Loosely(t, vm.Drained, should.BeTrue)
					})

					t.Run("deleted", func(t *ftt.Test) {
						rsp, err := srv.Delete(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp, should.Resemble(&emptypb.Empty{}))
						assert.Loosely(t, datastore.Get(c, vm), should.Resemble(datastore.ErrNoSuchEntity))
					})
				})

				t.Run("hostname", func(t *ftt.Test) {
					req := &instances.DeleteRequest{
						Hostname: "hostname",
					}

					t.Run("deletes", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, vm), should.BeNil)
						rsp, err := srv.Delete(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp, should.Resemble(&emptypb.Empty{}))
						assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
						assert.Loosely(t, vm.Drained, should.BeTrue)
					})

					t.Run("deleted", func(t *ftt.Test) {
						rsp, err := srv.Delete(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp, should.Resemble(&emptypb.Empty{}))
						assert.Loosely(t, datastore.Get(c, vm), should.Resemble(datastore.ErrNoSuchEntity))
					})
				})
			})
		})

		t.Run("List", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("filter", func(t *ftt.Test) {
					req := &instances.ListRequest{
						Filter: "filter",
					}
					_, err := srv.List(c, req)
					assert.Loosely(t, err, should.ErrLike("invalid filter expression"))
				})

				t.Run("page token", func(t *ftt.Test) {
					req := &instances.ListRequest{
						PageToken: "token",
					}
					_, err := srv.List(c, req)
					assert.Loosely(t, err, should.ErrLike("invalid page token"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						rsp, err := srv.List(c, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						vm := &model.VM{
							ID: "id",
						}
						assert.Loosely(t, datastore.Put(c, vm), should.BeNil)

						rsp, err := srv.List(c, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(1))
						assert.Loosely(t, rsp.Instances[0].Id, should.Equal("id"))
					})
				})

				t.Run("empty", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						req := &instances.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						vm := &model.VM{
							ID: "id",
						}
						assert.Loosely(t, datastore.Put(c, vm), should.BeNil)

						req := &instances.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(1))
						assert.Loosely(t, rsp.Instances[0].Id, should.Equal("id"))
					})
				})

				t.Run("pages", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.VM{ID: "id1"}), should.BeNil)
					assert.Loosely(t, datastore.Put(c, &model.VM{ID: "id2"}), should.BeNil)
					assert.Loosely(t, datastore.Put(c, &model.VM{ID: "id3"}), should.BeNil)

					t.Run("default", func(t *ftt.Test) {
						req := &instances.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.NotBeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						req := &instances.ListRequest{
							PageSize: 1,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(1))
						assert.Loosely(t, rsp.Instances[0].Id, should.Equal("id1"))
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(1))
						assert.Loosely(t, rsp.Instances[0].Id, should.Equal("id2"))
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(1))
						assert.Loosely(t, rsp.Instances[0].Id, should.Equal("id3"))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})

					t.Run("two", func(t *ftt.Test) {
						req := &instances.ListRequest{
							PageSize: 2,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(2))
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(1))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})

					t.Run("many", func(t *ftt.Test) {
						req := &instances.ListRequest{
							PageSize: 200,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Instances, should.HaveLength(3))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})
				})

				t.Run("filter", func(t *ftt.Test) {
					vm := &model.VM{
						ID: "id",
						AttributesIndexed: []string{
							"disk.image:image2",
						},
					}
					assert.Loosely(t, datastore.Put(c, vm), should.BeNil)

					req := &instances.ListRequest{
						Filter: "instances.disks.image=image1",
					}
					rsp, err := srv.List(c, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.Instances, should.BeEmpty)

					req.Filter = "instances.disks.image=image2"
					rsp, err = srv.List(c, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.Instances, should.HaveLength(1))
				})

				t.Run("prefix", func(t *ftt.Test) {
					vm := &model.VM{
						ID:     "id1",
						Prefix: "prefix1",
					}
					assert.Loosely(t, datastore.Put(c, vm), should.BeNil)
					vm = &model.VM{
						ID:     "id2",
						Prefix: "prefix2",
					}
					assert.Loosely(t, datastore.Put(c, vm), should.BeNil)

					req := &instances.ListRequest{
						Prefix: "prefix1",
					}
					rsp, err := srv.List(c, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.Instances, should.HaveLength(1))
					assert.Loosely(t, rsp.Instances[0].Id, should.Equal("id1"))
				})
			})
		})
	})
}
