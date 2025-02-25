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

package buildstatus

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func update(ctx context.Context, u *Updater) (*model.BuildStatus, error) {
	var bs *model.BuildStatus
	txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var err error
		bs, err = u.Do(ctx)
		return err
	}, nil)
	return bs, txErr
}
func TestUpdate(t *testing.T) {
	t.Parallel()
	ftt.Run("Update", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = txndefer.FilterRDS(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("fail", func(t *ftt.Test) {

			t.Run("not in transaction", func(t *ftt.Test) {
				u := &Updater{}
				_, err := u.Do(ctx)
				assert.Loosely(t, err, should.ErrLike("must update build status in a transaction"))
			})

			t.Run("update ended build", func(t *ftt.Test) {
				b := &model.Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_SUCCESS,
					},
					Status: pb.Status_SUCCESS,
				}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				u := &Updater{
					Build:       b,
					BuildStatus: &StatusWithDetails{Status: pb.Status_SUCCESS},
				}
				_, err := update(ctx, u)
				assert.Loosely(t, err, should.ErrLike("cannot update status for an ended build"))
			})

			t.Run("output status and task status", func(t *ftt.Test) {
				b := &model.Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_SCHEDULED,
					},
					Status: pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				u := &Updater{
					Build:        b,
					OutputStatus: &StatusWithDetails{Status: pb.Status_SUCCESS},
					TaskStatus:   &StatusWithDetails{Status: pb.Status_SUCCESS},
				}
				_, err := update(ctx, u)
				assert.Loosely(t, err, should.ErrLike("impossible: update build output status and task status at the same time"))
			})

			t.Run("nothing is provided to update", func(t *ftt.Test) {
				b := &model.Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_SCHEDULED,
					},
					Status: pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				u := &Updater{
					Build: b,
				}
				_, err := update(ctx, u)
				assert.Loosely(t, err, should.ErrLike("cannot set a build status to UNSPECIFIED"))
			})

			t.Run("BuildStatus not found", func(t *ftt.Test) {
				b := &model.Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_SCHEDULED,
					},
					Status: pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				u := &Updater{
					Build:       b,
					BuildStatus: &StatusWithDetails{Status: pb.Status_SUCCESS},
				}
				_, err := update(ctx, u)
				assert.Loosely(t, err, should.ErrLike("not found"))
			})
		})

		t.Run("pass", func(t *ftt.Test) {

			b := &model.Build{
				ID: 87654321,
				Proto: &pb.Build{
					Id: 87654321,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Output: &pb.Build_Output{},
					Status: pb.Status_SCHEDULED,
				},
				Status: pb.Status_SCHEDULED,
				CustomMetrics: []model.CustomMetric{
					{
						Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT,
						Metric: &pb.CustomMetricDefinition{
							Name:       "chrome/infra/custom/builds/failure_count_1",
							Predicates: []string{`build.tags.get_value("os")!=""`},
						},
					},
					{
						Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT,
						Metric: &pb.CustomMetricDefinition{
							Name:       "chrome/infra/custom/builds/failure_count_2",
							Predicates: []string{`build.tags.get_value("os")==""`},
						},
					},
				},
			}
			bk := datastore.KeyForObj(ctx, b)
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_SCHEDULED,
			}
			assert.Loosely(t, datastore.Put(ctx, b, bs), should.BeNil)
			updatedStatus := b.Proto.Status
			updatedStatusDetails := b.Proto.StatusDetails
			u := &Updater{
				Build: b,
				PostProcess: func(c context.Context, bld *model.Build, inf *model.BuildInfra) error {
					updatedStatus = bld.Proto.Status
					updatedStatusDetails = bld.Proto.StatusDetails
					return nil
				},
			}

			t.Run("direct update on build status ignore sub status", func(t *ftt.Test) {
				u.BuildStatus = &StatusWithDetails{Status: pb.Status_STARTED}
				u.OutputStatus = &StatusWithDetails{Status: pb.Status_SUCCESS} // only for test, impossible in practice
				bs, err := update(ctx, u)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, bs.Status, should.Equal(pb.Status_STARTED))
				assert.Loosely(t, updatedStatus, should.Equal(pb.Status_STARTED))
			})

			t.Run("update output status", func(t *ftt.Test) {
				t.Run("start, so build status is updated", func(t *ftt.Test) {
					u.OutputStatus = &StatusWithDetails{Status: pb.Status_STARTED}
					bs, err := update(ctx, u)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, bs.Status, should.Equal(pb.Status_STARTED))
					assert.Loosely(t, updatedStatus, should.Equal(pb.Status_STARTED))
				})

				t.Run("end, so build status is unchanged", func(t *ftt.Test) {
					u.OutputStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
					bs, err := update(ctx, u)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, bs, should.BeNil)
					assert.Loosely(t, updatedStatus, should.Equal(pb.Status_SCHEDULED))
				})
			})

			t.Run("update task status", func(t *ftt.Test) {
				t.Run("end, so build status is updated", func(t *ftt.Test) {
					b.Proto.Output.Status = pb.Status_SUCCESS
					assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
					u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
					bs, err := update(ctx, u)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, bs.Status, should.Equal(pb.Status_SUCCESS))
					assert.Loosely(t, updatedStatus, should.Equal(pb.Status_SUCCESS))
				})

				t.Run("start, so build status is unchanged", func(t *ftt.Test) {
					b.Proto.Output.Status = pb.Status_STARTED
					assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
					u.TaskStatus = &StatusWithDetails{Status: pb.Status_STARTED}
					bs, err := update(ctx, u)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, bs, should.BeNil)
					assert.Loosely(t, updatedStatus, should.Equal(pb.Status_SCHEDULED))
				})

				t.Run("final status based on both statuses", func(t *ftt.Test) {
					// output status is from the build entity.
					t.Run("output status not ended when task status success", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_STARTED
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, b.CustomBuilderConsecutiveFailuresMetrics, should.Match([]string{"chrome/infra/custom/builds/failure_count_2"}))
					})
					t.Run("output status not ended when task status success with SucceedBuildIfTaskSucceeded true", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_STARTED
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.SucceedBuildIfTaskSucceeded = true
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_SUCCESS))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_SUCCESS))
					})
					t.Run("output status not ended when task status fail", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_STARTED
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
							},
						}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_INFRA_FAILURE))
					})
					t.Run("output status SUCCESS, task status FAILURE", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_SUCCESS
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_FAILURE}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_FAILURE))
					})
					t.Run("output status SUCCESS, task status Status_INFRA_FAILURE", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_SUCCESS
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_INFRA_FAILURE}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_INFRA_FAILURE))
					})
					t.Run("output status FAILURE, task status PASS", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_FAILURE
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_FAILURE))
					})
					t.Run("output status FAILURE, task status INFRA_FAILURE", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_FAILURE
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
							},
						}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_FAILURE))
					})
					t.Run("output status CANCEL intentially, task status INFRA_FAILURE", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_CANCELED
						b.Proto.CancelTime = timestamppb.New(testclock.TestRecentTimeLocal)
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								Timeout: &pb.StatusDetails_Timeout{},
							},
						}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_CANCELED))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_CANCELED))
					})
					t.Run("output status CANCEL unintentially, task status INFRA_FAILURE", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_CANCELED
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								Timeout: &pb.StatusDetails_Timeout{},
							},
						}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_INFRA_FAILURE))
					})
					t.Run("output status CANCEL unintentially, task status SUCCESS", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_CANCELED
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_SUCCESS,
						}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_INFRA_FAILURE))
					})
					t.Run("both infra_failure with different details", func(t *ftt.Test) {
						b.Proto.Output.Status = pb.Status_INFRA_FAILURE
						b.Proto.Output.StatusDetails = &pb.StatusDetails{
							Timeout: &pb.StatusDetails_Timeout{},
						}
						assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
							},
						}
						bs, err := update(ctx, u)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatus, should.Equal(pb.Status_INFRA_FAILURE))
						assert.Loosely(t, updatedStatusDetails, should.Match(&pb.StatusDetails{
							Timeout: &pb.StatusDetails_Timeout{},
						}))
					})
				})
			})
		})
	})
}
