// Copyright 2022 The LUCI Authors.
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

package bbfake

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/buildbucket/appengine/model"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
)

func TestGetBuild(t *testing.T) {
	ftt.Run("GetBuild", t, func(t *ftt.Test) {
		fake := &Fake{}
		ctx := context.Background()
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)
		client := fake.MustNewClient(ctx, bbHost, lProject)

		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}

		fake.AddBuilder(bbHost, builderID, nil)
		build, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderID,
		})
		assert.NoErr(t, err)

		t.Run("Can get", func(t *ftt.Test) {
			t.Run("Without mask", func(t *ftt.Test) {
				expected := proto.Clone(build).(*bbpb.Build)
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: build.GetId(),
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(trimmedBuildWithDefaultMask(t, expected)))
			})
			t.Run("With mask", func(t *ftt.Test) {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: build.GetId(),
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "builder"},
						},
					},
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&bbpb.Build{
					Id:      build.GetId(),
					Builder: builderID,
				}))
			})
		})

		t.Run("No ACL", func(t *ftt.Test) {
			client := fake.MustNewClient(ctx, bbHost, "anotherProj")
			res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: build.GetId(),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, res, should.BeNil)
		})
		t.Run("Build not exist", func(t *ftt.Test) {
			res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: 777,
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Invalid input", func(t *ftt.Test) {
			t.Run("Zero build id", func(t *ftt.Test) {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id: 0,
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("Builder + build number", func(t *ftt.Test) {
				res, err := client.GetBuild(ctx, &bbpb.GetBuildRequest{
					Id:      build.GetId(),
					Builder: builderID,
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unimplemented))
				assert.Loosely(t, res, should.BeNil)
			})
		})
	})
}

func TestSearchBuild(t *testing.T) {
	ftt.Run("Search", t, func(t *ftt.Test) {
		fake := &Fake{}
		ctx := context.Background()
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)
		client := fake.MustNewClient(ctx, bbHost, lProject)

		const (
			gHost            = "example-review.googlesource.com"
			gRepo            = "repo/example"
			gChangeNum       = 546
			gPatchset        = 6
			gMinEquiPatchset = 2
		)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		gc := &bbpb.GerritChange{
			Host:     gHost,
			Project:  gRepo,
			Change:   gChangeNum,
			Patchset: gPatchset,
		}

		fake.AddBuilder(bbHost, builderID, nil)
		build, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder:       builderID,
			GerritChanges: []*bbpb.GerritChange{gc},
		})
		assert.NoErr(t, err)

		t.Run("Empty Predicate", func(t *ftt.Test) {
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&bbpb.SearchBuildsResponse{
				Builds: []*bbpb.Build{trimmedBuildWithDefaultMask(t, build)},
			}))
		})

		t.Run("Error on not allowed predicate", func(t *ftt.Test) {
			_, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
				Predicate: &bbpb.BuildPredicate{
					CreatedBy: "foo",
				},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Can apply Gerrit Change predicate", func(t *ftt.Test) {
			t.Run("Match", func(t *ftt.Test) {
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						GerritChanges: []*bbpb.GerritChange{gc},
					},
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&bbpb.SearchBuildsResponse{
					Builds: []*bbpb.Build{trimmedBuildWithDefaultMask(t, build)},
				}))
			})
			t.Run("Mismatch change", func(t *ftt.Test) {
				gc := proto.Clone(gc).(*bbpb.GerritChange)
				gc.Change += 2
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						GerritChanges: []*bbpb.GerritChange{gc},
					},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetBuilds(), should.BeEmpty)
				assert.Loosely(t, res.GetNextPageToken(), should.BeEmpty)
			})
			t.Run("Extra change", func(t *ftt.Test) {
				anotherGC := proto.Clone(gc).(*bbpb.GerritChange)
				anotherGC.Change += 2
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						GerritChanges: []*bbpb.GerritChange{gc, anotherGC},
					},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetBuilds(), should.BeEmpty)
				assert.Loosely(t, res.GetNextPageToken(), should.BeEmpty)
			})
		})

		t.Run("Can apply experimental predicate", func(t *ftt.Test) {
			build = fake.MutateBuild(ctx, bbHost, build.GetId(), func(b *bbpb.Build) {
				if b.Input == nil {
					b.Input = &bbpb.Build_Input{}
				}
				b.Input.Experimental = true
				b.Input.Experiments = append(b.Input.Experiments, "luci.non_production")
			})

			t.Run("Include experimental", func(t *ftt.Test) {
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{
						IncludeExperimental: true,
					},
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&bbpb.SearchBuildsResponse{
					Builds: []*bbpb.Build{trimmedBuildWithDefaultMask(t, build)},
				}))
			})
			t.Run("Not include experimental", func(t *ftt.Test) {
				res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
					Predicate: &bbpb.BuildPredicate{},
				})
				assert.NoErr(t, err)
				assert.Loosely(t, res.GetBuilds(), should.BeEmpty)
				assert.Loosely(t, res.GetNextPageToken(), should.BeEmpty)
			})
		})

		t.Run("Different host", func(t *ftt.Test) {
			client = fake.MustNewClient(ctx, "another-bb.example.com", lProject)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetBuilds(), should.BeEmpty)
			assert.Loosely(t, res.GetNextPageToken(), should.BeEmpty)
		})

		t.Run("Different project", func(t *ftt.Test) {
			anotherBuilder := &bbpb.BuilderID{
				Project: "anotherProj",
				Bucket:  "someBucket",
				Builder: "someBuilder",
			}
			fake.AddBuilder(bbHost, anotherBuilder, nil)
			_, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder:       anotherBuilder,
				GerritChanges: []*bbpb.GerritChange{gc},
			})
			assert.NoErr(t, err)

			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{})
			assert.NoErr(t, err)
			assert.Loosely(t, res.GetBuilds(), should.HaveLength(1))
			assert.Loosely(t, res.GetNextPageToken(), should.BeEmpty)
		})

		t.Run("Apply customized mask", func(t *ftt.Test) {
			fields, err := fieldmaskpb.New(build, "id")
			assert.NoErr(t, err)
			res, err := client.SearchBuilds(ctx, &bbpb.SearchBuildsRequest{
				Mask: &bbpb.BuildMask{
					Fields: fields,
				},
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&bbpb.SearchBuildsResponse{
				Builds: []*bbpb.Build{{Id: build.GetId()}},
			}))
		})

		t.Run("Paging", func(t *ftt.Test) {
			const totalBuilds = 50
			// there's already one build in the store.
			for i := 1; i < totalBuilds; i++ {
				_, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:       builderID,
					GerritChanges: []*bbpb.GerritChange{gc},
				})
				assert.NoErr(t, err)
			}

			for _, pageSize := range []int{0, 20} { // 0 means  default size
				buildCnt := 0
				t.Run(fmt.Sprintf("With size %d", pageSize), func(t *ftt.Test) {
					actualPageSize := pageSize
					if actualPageSize == 0 {
						actualPageSize = defaultSearchPageSize
					}
					req := &bbpb.SearchBuildsRequest{
						PageSize: int32(pageSize),
					}
					for {
						res, err := client.SearchBuilds(ctx, req)
						assert.NoErr(t, err)
						assert.Loosely(t, len(res.GetBuilds()), should.BeLessThanOrEqual(actualPageSize))
						buildCnt += len(res.GetBuilds())
						if res.NextPageToken == "" {
							break
						}
						req.PageToken = res.NextPageToken
					}
					assert.Loosely(t, buildCnt, should.Equal(totalBuilds))
				})
			}
		})
	})
}

func TestCancelBuild(t *testing.T) {
	ftt.Run("CancelBuild", t, func(t *ftt.Test) {
		fake := &Fake{}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)

		client := fake.MustNewClient(ctx, bbHost, lProject)

		tc.Add(1 * time.Minute)
		builderID := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "testBuilder",
		}
		fake.AddBuilder(bbHost, builderID, nil)
		build, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderID,
		})
		assert.NoErr(t, err)

		t.Run("Can cancel", func(t *ftt.Test) {
			t.Run("Without mask", func(t *ftt.Test) {
				res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
					Id:              build.GetId(),
					SummaryMarkdown: "no longer needed",
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(trimmedBuildWithDefaultMask(t, NewBuildConstructor().
					WithID(build.GetId()).
					WithHost(bbHost).
					WithBuilderID(builderID).
					WithStatus(bbpb.Status_CANCELED).
					WithCreateTime(tc.Now()).
					WithStartTime(tc.Now()).
					WithEndTime(tc.Now()).
					WithUpdateTime(tc.Now()).
					WithSummaryMarkdown("no longer needed").
					Construct())))
			})
			t.Run("With mask", func(t *ftt.Test) {
				res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
					Id:              build.GetId(),
					SummaryMarkdown: "no longer needed",
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "status"},
						},
					},
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&bbpb.Build{
					Id:     build.GetId(),
					Status: bbpb.Status_CANCELED,
				}))
			})
		})

		t.Run("No ACL", func(t *ftt.Test) {
			client := fake.MustNewClient(ctx, bbHost, "anotherProj")
			res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
				Id:              build.GetId(),
				SummaryMarkdown: "no longer needed",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Build not exist", func(t *ftt.Test) {
			res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
				Id: 777,
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Noop for build end with", func(t *ftt.Test) {
			for _, status := range []bbpb.Status{
				bbpb.Status_SUCCESS,
				bbpb.Status_FAILURE,
				bbpb.Status_INFRA_FAILURE,
				bbpb.Status_CANCELED,
			} {
				t.Run(status.String(), func(t *ftt.Test) {
					epoch := tc.Now()
					tc.Add(3 * time.Minute)
					fake.MutateBuild(ctx, bbHost, build.GetId(), func(b *bbpb.Build) {
						b.Status = status
						b.SummaryMarkdown = "ended already"
						b.StartTime = timestamppb.New(epoch.Add(1 * time.Minute))
						b.EndTime = timestamppb.New(epoch.Add(2 * time.Minute))
					})
					tc.Add(2 * time.Minute)
					res, err := client.CancelBuild(ctx, &bbpb.CancelBuildRequest{
						Id:              build.GetId(),
						SummaryMarkdown: "no longer needed",
						Mask: &bbpb.BuildMask{
							Fields: &fieldmaskpb.FieldMask{
								Paths: []string{"id", "status", "summary_markdown", "end_time"},
							},
						},
					})
					assert.NoErr(t, err)
					assert.That(t, res, should.Match(&bbpb.Build{
						Id:              build.GetId(),
						Status:          status,
						EndTime:         timestamppb.New(epoch.Add(2 * time.Minute)),
						SummaryMarkdown: "ended already",
					}))
				})
			}
		})
	})
}

func TestScheduleBuild(t *testing.T) {
	ftt.Run("ScheduleBuild", t, func(t *ftt.Test) {
		fake := &Fake{}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)
		builderNoProp := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "builderNoProp",
		}
		builderWithProp := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "builderWithProp",
		}
		fake.AddBuilder(bbHost, builderNoProp, nil)
		fake.AddBuilder(bbHost, builderWithProp,
			map[string]any{"foo": "bar"},
		)

		client := fake.MustNewClient(ctx, bbHost, lProject)
		t.Run("Can schedule", func(t *ftt.Test) {
			t.Run("Simple", func(t *ftt.Test) {
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder: builderNoProp,
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(trimmedBuildWithDefaultMask(t, &bbpb.Build{
					Id:         res.Id,
					Builder:    builderNoProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					UpdateTime: timestamppb.New(tc.Now()),
					Input:      &bbpb.Build_Input{},
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
				})))
			})
			t.Run("With additional args", func(t *ftt.Test) {
				props, err := structpb.NewStruct(map[string]any{
					"coolKey": "coolVal",
				})
				assert.NoErr(t, err)
				gc := &bbpb.GerritChange{
					Host:     "example-review.com",
					Project:  "testProj",
					Change:   1,
					Patchset: 2,
				}
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:       builderNoProp,
					Properties:    props,
					GerritChanges: []*bbpb.GerritChange{gc},
					Tags: []*bbpb.StringPair{
						{Key: "tagKey", Value: "tagValue"},
					},
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(trimmedBuildWithDefaultMask(t, &bbpb.Build{
					Id:         res.Id,
					Builder:    builderNoProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					UpdateTime: timestamppb.New(tc.Now()),
					Input: &bbpb.Build_Input{
						Properties:    props,
						GerritChanges: []*bbpb.GerritChange{gc},
					},
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							RequestedProperties: props,
							Hostname:            bbHost,
						},
					},
					Tags: []*bbpb.StringPair{
						{Key: "tagKey", Value: "tagValue"},
					},
				})))
			})
			t.Run("Override builder properties", func(t *ftt.Test) {
				props, err := structpb.NewStruct(map[string]any{
					"foo":  "not_bar",
					"cool": "awesome",
				})
				assert.NoErr(t, err)
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder:    builderWithProp,
					Properties: props,
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(trimmedBuildWithDefaultMask(t, &bbpb.Build{
					Id:         res.Id,
					Builder:    builderWithProp,
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: timestamppb.New(tc.Now()),
					UpdateTime: timestamppb.New(tc.Now()),
					Input: &bbpb.Build_Input{
						Properties: props,
					},
					Infra: &bbpb.BuildInfra{
						Buildbucket: &bbpb.BuildInfra_Buildbucket{
							RequestedProperties: props,
							Hostname:            bbHost,
						},
					},
				})))
			})
			t.Run("With mask", func(t *ftt.Test) {
				res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					Builder: builderNoProp,
					Mask: &bbpb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{"id", "status"},
						},
					},
				})
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&bbpb.Build{
					Id:     res.Id,
					Status: bbpb.Status_SCHEDULED,
				}))
			})
			t.Run("Decreasing build ID", func(t *ftt.Test) {
				const N = 10
				var prevBuildID int64 = math.MaxInt64
				for i := range N {
					builder := builderNoProp
					if i%2 == 1 {
						builder = builderWithProp
					}
					res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
						Builder: builder,
					})
					assert.NoErr(t, err)
					assert.Loosely(t, res.Id, should.BeLessThan(prevBuildID))
					prevBuildID = res.Id
				}
			})
			t.Run("Use requestID for deduplication", func(t *ftt.Test) {
				first, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					RequestId: "foo",
					Builder:   builderNoProp,
				})
				assert.NoErr(t, err)
				tc.Add(requestDeduplicationWindow / 2)
				dup, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					RequestId: "foo",
					Builder:   builderNoProp,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, dup.Id, should.Equal(first.Id))
				// Passes the deduplication window, should generate new build.
				tc.Add(requestDeduplicationWindow)
				newBuild, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
					RequestId: "foo",
					Builder:   builderNoProp,
				})
				assert.NoErr(t, err)
				assert.Loosely(t, newBuild.Id, should.NotEqual(first.Id))
			})
		})

		t.Run("Builder not found", func(t *ftt.Test) {
			res, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
				Builder: &bbpb.BuilderID{
					Project: lProject,
					Bucket:  "testBucket",
					Builder: "non-existent-builder",
				},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, res, should.BeNil)
		})
	})
}

func TestBatch(t *testing.T) {
	ftt.Run("Batch", t, func(t *ftt.Test) {
		fake := &Fake{}
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		const (
			bbHost   = "buildbucket.example.com"
			lProject = "testProj"
		)
		client := fake.MustNewClient(ctx, bbHost, lProject)

		builderFoo := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "foo",
		}
		builderBar := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "bar",
		}
		builderBaz := &bbpb.BuilderID{
			Project: lProject,
			Bucket:  "testBucket",
			Builder: "baz",
		}
		fake.AddBuilder(bbHost, builderFoo, nil)
		fake.AddBuilder(bbHost, builderBar, nil)
		fake.AddBuilder(bbHost, builderBaz, nil)

		buildFoo, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderFoo,
		})
		assert.NoErr(t, err)
		buildBar, err := client.ScheduleBuild(ctx, &bbpb.ScheduleBuildRequest{
			Builder: builderBar,
		})
		assert.NoErr(t, err)
		tc.Add(1 * time.Minute)
		buildBar = fake.MutateBuild(ctx, bbHost, buildBar.GetId(), func(b *bbpb.Build) {
			b.Status = bbpb.Status_SUCCESS
			b.StartTime = timestamppb.New(tc.Now().Add(-30 * time.Second))
			b.EndTime = timestamppb.New(tc.Now())
		})

		t.Run("Batch succeeds", func(t *ftt.Test) {
			res, err := client.Batch(ctx, &bbpb.BatchRequest{
				Requests: []*bbpb.BatchRequest_Request{
					{
						Request: &bbpb.BatchRequest_Request_CancelBuild{
							CancelBuild: &bbpb.CancelBuildRequest{
								Id:              buildFoo.GetId(),
								SummaryMarkdown: "no longer needed",
								Mask: &bbpb.BuildMask{
									Fields: &fieldmaskpb.FieldMask{
										Paths: []string{"id", "status", "summary_markdown"},
									},
								},
							},
						},
					},
					{
						Request: &bbpb.BatchRequest_Request_GetBuild{
							GetBuild: &bbpb.GetBuildRequest{
								Id: buildBar.GetId(),
							},
						},
					},
					{
						Request: &bbpb.BatchRequest_Request_ScheduleBuild{
							ScheduleBuild: &bbpb.ScheduleBuildRequest{
								Builder: builderBaz,
								Mask: &bbpb.BuildMask{
									Fields: &fieldmaskpb.FieldMask{
										Paths: []string{"builder", "status"},
									},
								},
							},
						},
					},
				},
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&bbpb.BatchResponse{
				Responses: []*bbpb.BatchResponse_Response{
					{
						Response: &bbpb.BatchResponse_Response_CancelBuild{
							CancelBuild: &bbpb.Build{
								Id:              buildFoo.Id,
								Status:          bbpb.Status_CANCELED,
								SummaryMarkdown: "no longer needed",
							},
						},
					},
					{
						Response: &bbpb.BatchResponse_Response_GetBuild{
							GetBuild: trimmedBuildWithDefaultMask(t, buildBar),
						},
					},
					{
						Response: &bbpb.BatchResponse_Response_ScheduleBuild{
							ScheduleBuild: &bbpb.Build{
								Builder: builderBaz,
								Status:  bbpb.Status_SCHEDULED,
							},
						},
					},
				},
			}))
		})

		t.Run("Batch fails", func(t *ftt.Test) {
			// ScheduleBuild in fake buildbucket will start from math.MaxInt - 1, so
			// there will never be a build in the fake with ID 12345 unless we
			// schedule numerous number of builds.
			const nonExistentBuildID = 12345
			res, err := client.Batch(ctx, &bbpb.BatchRequest{
				Requests: []*bbpb.BatchRequest_Request{
					{
						Request: &bbpb.BatchRequest_Request_CancelBuild{
							CancelBuild: &bbpb.CancelBuildRequest{
								Id:              buildFoo.GetId(),
								SummaryMarkdown: "no longer needed",
								Mask: &bbpb.BuildMask{
									Fields: &fieldmaskpb.FieldMask{
										Paths: []string{"id", "status", "summary_markdown"},
									},
								},
							},
						},
					},
					{
						Request: &bbpb.BatchRequest_Request_GetBuild{
							GetBuild: &bbpb.GetBuildRequest{
								Id: nonExistentBuildID,
							},
						},
					},
				},
			})
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&bbpb.BatchResponse{
				Responses: []*bbpb.BatchResponse_Response{
					{
						Response: &bbpb.BatchResponse_Response_CancelBuild{
							CancelBuild: &bbpb.Build{
								Id:              buildFoo.Id,
								Status:          bbpb.Status_CANCELED,
								SummaryMarkdown: "no longer needed",
							},
						},
					},
					{
						Response: &bbpb.BatchResponse_Response_Error{
							Error: status.Newf(codes.NotFound, "requested resource not found or \"project:%s\" does not have permission to view it", lProject).Proto(),
						},
					},
				},
			}))
		})
	})
}

func trimmedBuildWithDefaultMask(t testing.TB, b *bbpb.Build) *bbpb.Build {
	t.Helper()

	mask, err := model.NewBuildMask("", nil, nil)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	ret := proto.Clone(b).(*bbpb.Build)
	assert.Loosely(t, mask.Trim(ret), should.BeNil, truth.LineContext())
	return ret
}
