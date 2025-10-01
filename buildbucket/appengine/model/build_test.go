// Copyright 2020 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func mustStruct(data map[string]any) *structpb.Struct {
	ret, err := structpb.NewStruct(data)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestBuild(t *testing.T) {
	t.Parallel()

	ftt.Run("Build", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, tclock := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		t0 := tclock.Now()
		t0pb := timestamppb.New(t0)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		m := NoopBuildMask

		t.Run("read/write", func(t *ftt.Test) {
			cms := []CustomMetric{
				{
					Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED,
					Metric: &pb.CustomMetricDefinition{
						Name:       "custom_metric_created",
						Predicates: []string{`build.tags.get_value("os")!=""`},
						ExtraFields: map[string]string{
							"os":     `build.tags.get_value("os")`,
							"random": `"random"`,
						},
					},
				},
				{
					Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
					Metric: &pb.CustomMetricDefinition{
						Name:       "custom_metric_completed",
						Predicates: []string{`build.tags.get_value("os")!=""`},
						ExtraFields: map[string]string{
							"os":     `build.tags.get_value("os")`,
							"random": `"random"`,
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:      pb.Status_SUCCESS,
					CreateTime:  t0pb,
					UpdateTime:  t0pb,
					AncestorIds: []int64{2, 3, 4},
				},
				CustomMetrics: cms,
			}), should.BeNil)

			b := &Build{
				ID: 1,
			}
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			p := proto.Clone(b.Proto).(*pb.Build)
			b.Proto = &pb.Build{}
			b.NextBackendSyncTime = ""
			expectedCms := make([]CustomMetric, len(b.CustomMetrics))
			for i, cm := range b.CustomMetrics {
				expectedCms[i] = CustomMetric{
					Base:   cm.Base,
					Metric: proto.Clone(cm.Metric).(*pb.CustomMetricDefinition),
				}
			}
			b.CustomMetrics = nil
			assert.Loosely(t, b, should.Resemble(&Build{
				ID:                1,
				Proto:             &pb.Build{},
				BucketID:          "project/bucket",
				BuilderID:         "project/bucket/builder",
				Canary:            false,
				CreateTime:        datastore.RoundTime(t0),
				StatusChangedTime: datastore.RoundTime(t0),
				Experimental:      false,
				Incomplete:        false,
				Status:            pb.Status_SUCCESS,
				Project:           "project",
				LegacyProperties: LegacyProperties{
					Result: Success,
					Status: Completed,
				},
				AncestorIds: []int64{2, 3, 4},
				ParentID:    4,
			}))
			assert.Loosely(t, p, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:      pb.Status_SUCCESS,
				CreateTime:  t0pb,
				UpdateTime:  t0pb,
				AncestorIds: []int64{2, 3, 4},
			}))

			assert.Loosely(t, len(expectedCms), should.Equal(len(cms)))
			for i, cm := range expectedCms {
				assert.Loosely(t, cm.Base, should.Equal(cms[i].Base))
				assert.Loosely(t, cm.Metric, should.Resemble(cms[i].Metric))
			}
		})

		t.Run("legacy", func(t *ftt.Test) {
			t.Run("infra failure", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status:     pb.Status_INFRA_FAILURE,
						CreateTime: t0pb,
						UpdateTime: t0pb,
					},
				}), should.BeNil)

				b := &Build{
					ID: 1,
				}
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				b.NextBackendSyncTime = ""
				assert.Loosely(t, b, should.Resemble(&Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(t0),
					StatusChangedTime: datastore.RoundTime(t0),
					Experimental:      false,
					Incomplete:        false,
					Status:            pb.Status_INFRA_FAILURE,
					Project:           "project",
					LegacyProperties: LegacyProperties{
						FailureReason: InfraFailure,
						Result:        Failure,
						Status:        Completed,
					},
				}))
				assert.Loosely(t, p, should.Resemble(&pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     pb.Status_INFRA_FAILURE,
					CreateTime: t0pb,
					UpdateTime: t0pb,
				}))
			})

			t.Run("timeout", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_INFRA_FAILURE,
						StatusDetails: &pb.StatusDetails{
							Timeout: &pb.StatusDetails_Timeout{},
						},
						CreateTime: t0pb,
						UpdateTime: t0pb,
					},
				}), should.BeNil)

				b := &Build{
					ID: 1,
				}
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				b.NextBackendSyncTime = ""
				assert.Loosely(t, b, should.Resemble(&Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(t0),
					StatusChangedTime: datastore.RoundTime(t0),
					Experimental:      false,
					Incomplete:        false,
					Status:            pb.Status_INFRA_FAILURE,
					Project:           "project",
					LegacyProperties: LegacyProperties{
						CancelationReason: TimeoutCanceled,
						Result:            Canceled,
						Status:            Completed,
					},
				}))
				assert.Loosely(t, p, should.Resemble(&pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_INFRA_FAILURE,
					StatusDetails: &pb.StatusDetails{
						Timeout: &pb.StatusDetails_Timeout{},
					},
					CreateTime: t0pb,
					UpdateTime: t0pb,
				}))
			})

			t.Run("canceled", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status:     pb.Status_CANCELED,
						CreateTime: t0pb,
						UpdateTime: t0pb,
					},
				}), should.BeNil)

				b := &Build{
					ID: 1,
				}
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				p := proto.Clone(b.Proto).(*pb.Build)
				b.Proto = &pb.Build{}
				b.NextBackendSyncTime = ""
				assert.Loosely(t, b, should.Resemble(&Build{
					ID:                1,
					Proto:             &pb.Build{},
					BucketID:          "project/bucket",
					BuilderID:         "project/bucket/builder",
					Canary:            false,
					CreateTime:        datastore.RoundTime(t0),
					StatusChangedTime: datastore.RoundTime(t0),
					Experimental:      false,
					Incomplete:        false,
					Status:            pb.Status_CANCELED,
					Project:           "project",
					LegacyProperties: LegacyProperties{
						CancelationReason: ExplicitlyCanceled,
						Result:            Canceled,
						Status:            Completed,
					},
				}))
				assert.Loosely(t, p, should.Resemble(&pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     pb.Status_CANCELED,
					CreateTime: t0pb,
					UpdateTime: t0pb,
				}))
			})
		})

		t.Run("Realm", func(t *ftt.Test) {
			b := &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}
			assert.Loosely(t, b.Realm(), should.Equal("project:bucket"))
		})

		t.Run("ToProto", func(t *ftt.Test) {
			b := &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
				},
				Tags: []string{
					"key1:value1",
					"builder:hidden",
					"key2:value2",
				},
			}
			key := datastore.KeyForObj(ctx, b)
			assert.Loosely(t, datastore.Put(ctx, &BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &BuildInputProperties{
				Build: key,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"input": {
							Kind: &structpb.Value_StringValue{
								StringValue: "input value",
							},
						},
					},
				},
			}), should.BeNil)

			t.Run("mask", func(t *ftt.Test) {
				t.Run("include", func(t *ftt.Test) {
					m := HardcodedBuildMask("id")
					p, err := b.ToProto(ctx, m, nil)
					assert.NoErr(t, err)
					assert.Loosely(t, p.Id, should.Equal(1))
				})

				t.Run("exclude", func(t *ftt.Test) {
					m := HardcodedBuildMask("builder")
					p, err := b.ToProto(ctx, m, nil)
					assert.NoErr(t, err)
					assert.Loosely(t, p.Id, should.BeZero)
				})
			})

			t.Run("tags", func(t *ftt.Test) {
				p, err := b.ToProto(ctx, m, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, p.Tags, should.Resemble([]*pb.StringPair{
					{
						Key:   "key1",
						Value: "value1",
					},
					{
						Key:   "key2",
						Value: "value2",
					},
				}))
				assert.Loosely(t, b.Proto.Tags, should.BeEmpty)
			})

			t.Run("infra", func(t *ftt.Test) {
				p, err := b.ToProto(ctx, m, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, p.Infra, should.Resemble(&pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "example.com",
					},
				}))
				assert.Loosely(t, b.Proto.Infra, should.BeNil)
			})

			t.Run("input properties", func(t *ftt.Test) {
				p, err := b.ToProto(ctx, m, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, p.Input.Properties, should.Resemble(mustStruct(map[string]any{
					"input": "input value",
				})))
				assert.Loosely(t, b.Proto.Input, should.BeNil)
			})

			t.Run("output properties", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &BuildOutputProperties{
					Build: key,
					Proto: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"output": {
								Kind: &structpb.Value_StringValue{
									StringValue: "output value",
								},
							},
						},
					},
				}), should.BeNil)
				p, err := b.ToProto(ctx, m, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, p.Output.Properties, should.Resemble(mustStruct(map[string]any{
					"output": "output value",
				})))
				assert.Loosely(t, b.Proto.Output, should.BeNil)

				t.Run("one missing, one found", func(t *ftt.Test) {
					b1 := &pb.Build{
						Id: 1,
					}
					b2 := &pb.Build{
						Id: 2,
					}
					m := HardcodedBuildMask("output.properties")
					assert.Loosely(t, LoadBuildDetails(ctx, m, nil, b1, b2), should.BeNil)
					assert.Loosely(t, b1.Output.Properties, should.Resemble(mustStruct(map[string]any{
						"output": "output value",
					})))
					assert.Loosely(t, b2.Output.GetProperties(), should.BeNil)
				})
			})

			t.Run("output properties(large)", func(t *ftt.Test) {
				largeProps, err := structpb.NewStruct(map[string]any{})
				assert.NoErr(t, err)
				k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
				v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
				for i := range 10000 {
					largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: v,
						},
					}
				}
				outProp := &BuildOutputProperties{
					Build: key,
					Proto: largeProps,
				}
				assert.Loosely(t, outProp.Put(ctx), should.BeNil)

				p, err := b.ToProto(ctx, m, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, p.Output.Properties, should.Resemble(largeProps))
				assert.Loosely(t, b.Proto.Output, should.BeNil)
			})

			t.Run("steps", func(t *ftt.Test) {
				s, err := proto.Marshal(&pb.Build{
					Steps: []*pb.Step{
						{
							Name: "step",
						},
					},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, datastore.Put(ctx, &BuildSteps{
					Build:    key,
					Bytes:    s,
					IsZipped: false,
				}), should.BeNil)
				p, err := b.ToProto(ctx, m, nil)
				assert.NoErr(t, err)
				assert.Loosely(t, p.Steps, should.Resemble([]*pb.Step{
					{
						Name: "step",
					},
				}))
				assert.Loosely(t, b.Proto.Steps, should.BeEmpty)
			})
		})

		t.Run("ToSimpleBuildProto", func(t *ftt.Test) {
			b := &Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Tags: []*pb.StringPair{
						{
							Key:   "k1",
							Value: "v1",
						},
					},
				},
				Project:   "project",
				BucketID:  "project/bucket",
				BuilderID: "project/bucket/builder",
				Tags: []string{
					"k1:v1",
				},
			}

			actual := b.ToSimpleBuildProto(ctx)
			assert.Loosely(t, actual, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Tags: []*pb.StringPair{
					{
						Key:   "k1",
						Value: "v1",
					},
				},
			}))
		})

		t.Run("ExperimentsString", func(t *ftt.Test) {
			b := &Build{}
			check := func(exps []string, enabled string) {
				b.Experiments = exps
				assert.Loosely(t, b.ExperimentsString(), should.Equal(enabled))
			}

			t.Run("Returns None", func(t *ftt.Test) {
				check([]string{}, "None")
			})

			t.Run("Sorted", func(t *ftt.Test) {
				exps := []string{"+exp4", "-exp3", "+exp1", "-exp10"}
				check(exps, "exp1|exp4")
			})
		})

		t.Run("NextBackendSyncTime", func(t *ftt.Test) {
			b := &Build{
				ID:      1,
				Project: "project",
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:      pb.Status_STARTED,
					CreateTime:  t0pb,
					UpdateTime:  t0pb,
					AncestorIds: []int64{2, 3, 4},
				},
				BackendTarget: "backend",
			}
			b.GenerateNextBackendSyncTime(ctx, 1)
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)

			// First save.
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			ut0 := b.NextBackendSyncTime
			parts := strings.Split(ut0, syncTimeSep)
			assert.Loosely(t, parts, should.HaveLength(4))
			assert.Loosely(t, parts[3], should.Equal(fmt.Sprint(t0.Add(b.BackendSyncInterval).Truncate(time.Minute).Unix())))
			assert.Loosely(t, ut0, should.Equal("backend--project--0--1454472600"))

			// update soon after, NextBackendSyncTime unchanged.
			b.Proto.UpdateTime = timestamppb.New(t0.Add(time.Second))
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			assert.Loosely(t, b.NextBackendSyncTime, should.Equal(ut0))
			assert.Loosely(t, b.BackendSyncInterval, should.Equal(defaultBuildSyncInterval))

			// update after 30sec, NextBackendSyncTime unchanged.
			b.Proto.UpdateTime = timestamppb.New(t0.Add(40 * time.Second))
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			assert.Loosely(t, b.NextBackendSyncTime, should.BeGreaterThan(ut0))

			// update after 2min, NextBackendSyncTime changed.
			t1 := t0.Add(2 * time.Minute)
			b.Proto.UpdateTime = timestamppb.New(t1)
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			assert.Loosely(t, b.NextBackendSyncTime, should.BeGreaterThan(ut0))
			parts = strings.Split(b.NextBackendSyncTime, syncTimeSep)
			assert.Loosely(t, parts, should.HaveLength(4))
			assert.Loosely(t, parts[3], should.Equal(fmt.Sprint(t1.Add(b.BackendSyncInterval).Truncate(time.Minute).Unix())))
		})

		t.Run("EvaluateBuildForCustomBuilderMetrics", func(t *ftt.Test) {
			t.Run("nothing to check", func(t *ftt.Test) {
				blds := []*Build{
					// no custom metrics.
					{
						ID: 1,
						Proto: &pb.Build{
							Status: pb.Status_SCHEDULED,
						},
					},
					// no custom metrics of requested base.
					{
						ID: 2,
						Proto: &pb.Build{
							Status: pb.Status_SCHEDULED,
						},
						CustomMetrics: []CustomMetric{
							{
								Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED,
								Metric: &pb.CustomMetricDefinition{
									Name:       "custom_metric_created",
									Predicates: []string{`build.tags.get_value("os")!=""`},
									ExtraFields: map[string]string{
										"os": `build.tags.get_value("os")`,
									},
								},
							},
						},
					},
					// requested metric already added to the build.
					{
						ID: 3,
						Proto: &pb.Build{
							Status: pb.Status_SCHEDULED,
						},
						CustomMetrics: []CustomMetric{
							{
								Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
								Metric: &pb.CustomMetricDefinition{
									Name:       "custom_metric_count",
									Predicates: []string{`build.tags.get_value("os")!=""`},
								},
							},
						},
						CustomBuilderCountMetrics: []string{"custom_metric_count"},
					},
				}
				for _, bld := range blds {
					err := EvaluateBuildForCustomBuilderMetrics(ctx, bld, bld.Proto, false)
					assert.NoErr(t, err)
				}
			})

			t.Run("failed", func(t *ftt.Test) {
				bld := &Build{
					ID: 3,
					Proto: &pb.Build{
						Status: pb.Status_SCHEDULED,
						Tags: []*pb.StringPair{
							{
								Key:   "os",
								Value: "mac",
							},
						},
					},
					CustomMetrics: []CustomMetric{
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name: "custom_metric_count",
							},
						},
					},
				}
				err := EvaluateBuildForCustomBuilderMetrics(ctx, bld, bld.Proto, false)
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run("re-evaluate", func(t *ftt.Test) {
				b := &Build{
					ID: 1,
					Proto: &pb.Build{
						Id:     1,
						Status: pb.Status_STARTED,
						Tags: []*pb.StringPair{
							{
								Key:   "os",
								Value: "mac",
							},
						},
					},
					CustomMetrics: []CustomMetric{
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name:       "custom_metric_count1",
								Predicates: []string{`build.tags.get_value("os")==""`},
							},
						},
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name:       "custom_metric_count2",
								Predicates: []string{`build.tags.get_value("os")!=""`},
							},
						},
					},
					CustomBuilderCountMetrics: []string{"custom_metric_count1"},
				}
				err := EvaluateBuildForCustomBuilderMetrics(ctx, b, b.Proto, false)
				assert.NoErr(t, err)
				assert.Loosely(t, b.CustomBuilderCountMetrics, should.Resemble([]string{"custom_metric_count2"}))
			})

			t.Run("re-evaluate with provided build proto", func(t *ftt.Test) {
				b := &Build{
					ID: 1,
					Proto: &pb.Build{
						Id:     1,
						Status: pb.Status_STARTED,
						Tags: []*pb.StringPair{
							{
								Key:   "os",
								Value: "mac",
							},
						},
					},
					CustomMetrics: []CustomMetric{
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name:       "custom_metric_count1",
								Predicates: []string{`build.tags.get_value("os")==""`},
							},
						},
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name:       "custom_metric_count2",
								Predicates: []string{`build.tags.get_value("os")!=""`},
							},
						},
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name:       "custom_metric_count3",
								Predicates: []string{`has(build.input.properties.key)`},
							},
						},
					},
					CustomBuilderCountMetrics: []string{"custom_metric_count1"},
				}

				bp := &pb.Build{
					Id:     1,
					Status: pb.Status_STARTED,
					Tags: []*pb.StringPair{
						{
							Key:   "os",
							Value: "mac",
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					},
				}
				err := EvaluateBuildForCustomBuilderMetrics(ctx, b, bp, false)
				assert.NoErr(t, err)
				assert.Loosely(t, b.CustomBuilderCountMetrics, should.Resemble([]string{"custom_metric_count2", "custom_metric_count3"}))
			})

			t.Run("loadDetails", func(t *ftt.Test) {
				b := &Build{
					ID: 1,
					Proto: &pb.Build{
						Id:     1,
						Status: pb.Status_SCHEDULED,
					},
					CustomMetrics: []CustomMetric{
						{
							Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
							Metric: &pb.CustomMetricDefinition{
								Name:       "custom_metric_count",
								Predicates: []string{`has(build.input.properties.input)`},
							},
						},
					},
				}
				key := datastore.KeyForObj(ctx, b)
				assert.Loosely(t, datastore.Put(ctx, &BuildInputProperties{
					Build: key,
					Proto: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"input": {
								Kind: &structpb.Value_StringValue{
									StringValue: "input value",
								},
							},
						},
					},
				}), should.BeNil)
				err := EvaluateBuildForCustomBuilderMetrics(ctx, b, nil, false)
				assert.NoErr(t, err)
				assert.Loosely(t, len(b.CustomBuilderCountMetrics), should.BeZero)
				err = EvaluateBuildForCustomBuilderMetrics(ctx, b, nil, true)
				assert.NoErr(t, err)
				assert.Loosely(t, b.CustomBuilderCountMetrics, should.Resemble([]string{"custom_metric_count"}))
			})
		})
	})
}
