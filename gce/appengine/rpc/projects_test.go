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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/projects/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	ftt.Run("Projects", t, func(t *ftt.Test) {
		srv := &Projects{}
		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		t.Run("List", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("page token", func(t *ftt.Test) {
					req := &projects.ListRequest{
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
						assert.Loosely(t, rsp.Projects, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						p := &model.Project{
							ID: "id",
						}
						assert.Loosely(t, datastore.Put(c, p), should.BeNil)

						rsp, err := srv.List(c, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Projects, should.HaveLength(1))
					})
				})

				t.Run("empty", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						req := &projects.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Projects, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						p := &model.Project{
							ID: "id",
						}
						assert.Loosely(t, datastore.Put(c, p), should.BeNil)

						req := &projects.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Projects, should.HaveLength(1))
					})
				})

				t.Run("pages", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(c, &model.Project{ID: "id1"}), should.BeNil)
					assert.Loosely(t, datastore.Put(c, &model.Project{ID: "id2"}), should.BeNil)
					assert.Loosely(t, datastore.Put(c, &model.Project{ID: "id3"}), should.BeNil)

					t.Run("default", func(t *ftt.Test) {
						req := &projects.ListRequest{}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.Projects, should.NotBeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						req := &projects.ListRequest{
							PageSize: 1,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
						assert.Loosely(t, rsp.Projects, should.HaveLength(1))

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
						assert.Loosely(t, rsp.Projects, should.HaveLength(1))

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
						assert.Loosely(t, rsp.Projects, should.HaveLength(1))
					})

					t.Run("two", func(t *ftt.Test) {
						req := &projects.ListRequest{
							PageSize: 2,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
						assert.Loosely(t, rsp.Projects, should.HaveLength(2))

						req.PageToken = rsp.NextPageToken
						rsp, err = srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
						assert.Loosely(t, rsp.Projects, should.HaveLength(1))
					})

					t.Run("many", func(t *ftt.Test) {
						req := &projects.ListRequest{
							PageSize: 200,
						}
						rsp, err := srv.List(c, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
						assert.Loosely(t, rsp.Projects, should.HaveLength(3))
					})
				})
			})
		})

		t.Run("Ensure", func(t *ftt.Test) {
			t.Run("Binary", func(t *ftt.Test) {
				req := &projects.EnsureRequest{
					Id: "id",
					Project: &projects.Config{
						Project: "project",
						Region: []string{
							"region1",
							"region2",
						},
						Revision: "revision-1",
					},
				}
				cfg, err := srv.Ensure(c, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.Resemble(&projects.Config{
					Project: "project",
					Region: []string{
						"region1",
						"region2",
					},
					Revision: "revision-1",
				}))
			})
		})
	})
}
