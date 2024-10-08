// Copyright 2023 The LUCI Authors.
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

package buildcron

import (
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestTriggerSyncBackendTasks(t *testing.T) {
	t.Parallel()
	ftt.Run("TriggerSyncBackendTasks", t, func(t *ftt.Test) {
		ctx, _, sch := setUp()

		b1 := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "p1",
				},
			},
		}
		b2 := &model.Build{
			ID:            1,
			BackendTarget: "b1",
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "p1",
				},
			},
		}
		b3 := &model.Build{
			ID:            3,
			BackendTarget: "b1",
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "p1",
				},
			},
		}
		b4 := &model.Build{
			ID:            4,
			BackendTarget: "b2",
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "p1",
				},
			},
		}
		b5 := &model.Build{
			ID:            5,
			BackendTarget: "b1",
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "p2",
				},
			},
		}
		b6 := &model.Build{
			ID:            6,
			BackendTarget: "b3",
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "p3",
				},
			},
		}

		assert.Loosely(t, datastore.Put(ctx, b1, b2, b3, b4, b5, b6), should.BeNil)
		assert.Loosely(t, TriggerSyncBackendTasks(ctx), should.BeNil)
		assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
		pairs := make([]*projectBackendPair, 4)
		for i, tsk := range sch.Tasks() {
			switch v := tsk.Payload.(type) {
			case *taskdefs.SyncBuildsWithBackendTasks:
				pairs[i] = &projectBackendPair{
					project: v.Project,
					backend: v.Backend,
				}
			default:
				panic("invalid task payload")
			}
		}
		sort.Slice(pairs, func(i, j int) bool {
			switch {
			case pairs[i].project < pairs[j].project:
				return true
			case pairs[i].project > pairs[j].project:
				return false
			default:
				return pairs[i].backend < pairs[j].backend
			}
		})

		expected := []*projectBackendPair{
			{
				project: "p1",
				backend: "b1",
			},
			{
				project: "p1",
				backend: "b2",
			},
			{
				project: "p2",
				backend: "b1",
			},
			{
				project: "p3",
				backend: "b3",
			},
		}
		assert.Loosely(t, pairs, should.Resemble(expected))
	})
}
