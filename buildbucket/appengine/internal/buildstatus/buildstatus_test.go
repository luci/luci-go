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
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	Convey("Update", t, func() {
		ctx := memory.Use(context.Background())
		ctx = txndefer.FilterRDS(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("fail", func() {

			Convey("not in transaction", func() {
				u := &Updater{}
				_, err := u.Do(ctx)
				So(err, ShouldErrLike, "must update build status in a transaction")
			})

			Convey("update ended build", func() {
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
				So(datastore.Put(ctx, b), ShouldBeNil)
				u := &Updater{
					Build:       b,
					BuildStatus: &StatusWithDetails{Status: pb.Status_SUCCESS},
				}
				_, err := update(ctx, u)
				So(err, ShouldErrLike, "cannot update status for an ended build")
			})

			Convey("output status and task status", func() {
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
				So(datastore.Put(ctx, b), ShouldBeNil)
				u := &Updater{
					Build:        b,
					OutputStatus: &StatusWithDetails{Status: pb.Status_SUCCESS},
					TaskStatus:   &StatusWithDetails{Status: pb.Status_SUCCESS},
				}
				_, err := update(ctx, u)
				So(err, ShouldErrLike, "impossible: update build output status and task status at the same time")
			})

			Convey("nothing is provided to update", func() {
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
				So(datastore.Put(ctx, b), ShouldBeNil)
				u := &Updater{
					Build: b,
				}
				_, err := update(ctx, u)
				So(err, ShouldErrLike, "cannot set a build status to UNSPECIFIED")
			})

			Convey("BuildStatus not found", func() {
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
				So(datastore.Put(ctx, b), ShouldBeNil)
				u := &Updater{
					Build:       b,
					BuildStatus: &StatusWithDetails{Status: pb.Status_SUCCESS},
				}
				_, err := update(ctx, u)
				So(err, ShouldErrLike, "not found")
			})
		})

		Convey("pass", func() {

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
			So(datastore.Put(ctx, b, bs), ShouldBeNil)
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

			Convey("direct update on build status ignore sub status", func() {
				u.BuildStatus = &StatusWithDetails{Status: pb.Status_STARTED}
				u.OutputStatus = &StatusWithDetails{Status: pb.Status_SUCCESS} // only for test, impossible in practice
				bs, err := update(ctx, u)
				So(err, ShouldBeNil)
				So(bs.Status, ShouldEqual, pb.Status_STARTED)
				So(updatedStatus, ShouldEqual, pb.Status_STARTED)
			})

			Convey("update output status", func() {
				Convey("start, so build status is updated", func() {
					u.OutputStatus = &StatusWithDetails{Status: pb.Status_STARTED}
					bs, err := update(ctx, u)
					So(err, ShouldBeNil)
					So(bs.Status, ShouldEqual, pb.Status_STARTED)
					So(updatedStatus, ShouldEqual, pb.Status_STARTED)
				})

				Convey("end, so build status is unchanged", func() {
					u.OutputStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
					bs, err := update(ctx, u)
					So(err, ShouldBeNil)
					So(bs, ShouldBeNil)
					So(updatedStatus, ShouldEqual, pb.Status_SCHEDULED)
				})
			})

			Convey("update task status", func() {
				Convey("end, so build status is updated", func() {
					b.Proto.Output.Status = pb.Status_SUCCESS
					So(datastore.Put(ctx, b), ShouldBeNil)
					u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
					bs, err := update(ctx, u)
					So(err, ShouldBeNil)
					So(bs.Status, ShouldEqual, pb.Status_SUCCESS)
					So(updatedStatus, ShouldEqual, pb.Status_SUCCESS)
				})

				Convey("start, so build status is unchanged", func() {
					b.Proto.Output.Status = pb.Status_STARTED
					So(datastore.Put(ctx, b), ShouldBeNil)
					u.TaskStatus = &StatusWithDetails{Status: pb.Status_STARTED}
					bs, err := update(ctx, u)
					So(err, ShouldBeNil)
					So(bs, ShouldBeNil)
					So(updatedStatus, ShouldEqual, pb.Status_SCHEDULED)
				})

				Convey("final status based on both statuses", func() {
					// output status is from the build entity.
					Convey("output status not ended when task status success", func() {
						b.Proto.Output.Status = pb.Status_STARTED
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(b.CustomBuilderConsecutiveFailuresMetrics, ShouldResemble, []string{"chrome/infra/custom/builds/failure_count_2"})
					})
					Convey("output status not ended when task status success with SucceedBuildIfTaskSucceeded true", func() {
						b.Proto.Output.Status = pb.Status_STARTED
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.SucceedBuildIfTaskSucceeded = true
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_SUCCESS)
						So(updatedStatus, ShouldEqual, pb.Status_SUCCESS)
					})
					Convey("output status not ended when task status fail", func() {
						b.Proto.Output.Status = pb.Status_STARTED
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
							},
						}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_INFRA_FAILURE)
					})
					Convey("output status SUCCESS, task status FAILURE", func() {
						b.Proto.Output.Status = pb.Status_SUCCESS
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_FAILURE}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_FAILURE)
					})
					Convey("output status SUCCESS, task status Status_INFRA_FAILURE", func() {
						b.Proto.Output.Status = pb.Status_SUCCESS
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_INFRA_FAILURE}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_INFRA_FAILURE)
					})
					Convey("output status FAILURE, task status PASS", func() {
						b.Proto.Output.Status = pb.Status_FAILURE
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{Status: pb.Status_SUCCESS}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_FAILURE)
					})
					Convey("output status FAILURE, task status INFRA_FAILURE", func() {
						b.Proto.Output.Status = pb.Status_FAILURE
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
							},
						}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_FAILURE)
					})
					Convey("output status CANCEL intentially, task status INFRA_FAILURE", func() {
						b.Proto.Output.Status = pb.Status_CANCELED
						b.Proto.CancelTime = timestamppb.New(testclock.TestRecentTimeLocal)
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								Timeout: &pb.StatusDetails_Timeout{},
							},
						}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_CANCELED)
						So(updatedStatus, ShouldEqual, pb.Status_CANCELED)
					})
					Convey("output status CANCEL unintentially, task status INFRA_FAILURE", func() {
						b.Proto.Output.Status = pb.Status_CANCELED
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								Timeout: &pb.StatusDetails_Timeout{},
							},
						}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_INFRA_FAILURE)
					})
					Convey("output status CANCEL unintentially, task status SUCCESS", func() {
						b.Proto.Output.Status = pb.Status_CANCELED
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_SUCCESS,
						}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_INFRA_FAILURE)
					})
					Convey("both infra_failure with different details", func() {
						b.Proto.Output.Status = pb.Status_INFRA_FAILURE
						b.Proto.Output.StatusDetails = &pb.StatusDetails{
							Timeout: &pb.StatusDetails_Timeout{},
						}
						So(datastore.Put(ctx, b), ShouldBeNil)
						u.TaskStatus = &StatusWithDetails{
							Status: pb.Status_INFRA_FAILURE,
							Details: &pb.StatusDetails{
								ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
							},
						}
						bs, err := update(ctx, u)
						So(err, ShouldBeNil)
						So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatus, ShouldEqual, pb.Status_INFRA_FAILURE)
						So(updatedStatusDetails, ShouldResembleProto, &pb.StatusDetails{
							Timeout: &pb.StatusDetails_Timeout{},
						})
					})
				})
			})
		})
	})
}
