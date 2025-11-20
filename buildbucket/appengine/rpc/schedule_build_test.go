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

package rpc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcStatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	luciCmProto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	rdbPb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/resultdb"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/rpc/testutil"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func fv(vs ...any) []any {
	ret := []any{"luci.project.bucket", "builder"}
	return append(ret, vs...)
}

func TestScheduleBuild(t *testing.T) {
	t.Parallel()

	// Note: request deduplication IDs depend on a hash of this value.
	const userID = identity.Identity("user:caller@example.com")

	ftt.Run("builderMatches", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			assert.Loosely(t, builderMatches("", nil), should.BeTrue)
			assert.Loosely(t, builderMatches("project/bucket/builder", nil), should.BeTrue)
		})

		t.Run("empty", func(t *ftt.Test) {
			p := &pb.BuilderPredicate{}
			assert.Loosely(t, builderMatches("", p), should.BeTrue)
			assert.Loosely(t, builderMatches("project/bucket/builder", p), should.BeTrue)
		})

		t.Run("regex", func(t *ftt.Test) {
			p := &pb.BuilderPredicate{
				Regex: []string{
					"project/bucket/.+",
				},
			}
			assert.Loosely(t, builderMatches("", p), should.BeFalse)
			assert.Loosely(t, builderMatches("project/bucket/builder", p), should.BeTrue)
			assert.Loosely(t, builderMatches("project/other/builder", p), should.BeFalse)
		})

		t.Run("regex exclude", func(t *ftt.Test) {
			p := &pb.BuilderPredicate{
				RegexExclude: []string{
					"project/bucket/.+",
				},
			}
			assert.Loosely(t, builderMatches("", p), should.BeTrue)
			assert.Loosely(t, builderMatches("project/bucket/builder", p), should.BeFalse)
			assert.Loosely(t, builderMatches("project/other/builder", p), should.BeTrue)
		})

		t.Run("regex exclude > regex", func(t *ftt.Test) {
			p := &pb.BuilderPredicate{
				Regex: []string{
					"project/bucket/.+",
				},
				RegexExclude: []string{
					"project/bucket/builder",
				},
			}
			assert.Loosely(t, builderMatches("", p), should.BeFalse)
			assert.Loosely(t, builderMatches("project/bucket/builder", p), should.BeFalse)
			assert.Loosely(t, builderMatches("project/bucket/other", p), should.BeTrue)
		})
	})

	ftt.Run("generateBuildNumbers", t, func(t *ftt.Test) {
		ctx := metrics.WithServiceInfo(memory.Use(context.Background()), "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("one", func(t *ftt.Test) {
			blds := []*model.Build{
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				},
			}
			err := generateBuildNumbers(ctx, blds)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, blds, should.Resemble([]*model.Build{
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Number: 1,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder/1",
					},
				},
			}))
		})

		t.Run("many", func(t *ftt.Test) {
			blds := []*model.Build{
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
					},
				},
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
			}
			err := generateBuildNumbers(ctx, blds)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, blds, should.Resemble([]*model.Build{
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Number: 1,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder1/1",
					},
				},
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder2",
						},
						Number: 1,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder2/1",
					},
				},
				{
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder1",
						},
						Number: 2,
					},
					Tags: []string{
						"build_address:luci.project.bucket/builder1/2",
					},
				},
			}))
		})
	})

	ftt.Run("scheduleRequestFromTemplate", t, func(t *ftt.Test) {
		ctx := metrics.WithServiceInfo(memory.Use(context.Background()), "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
			),
		})

		testutil.PutBucket(ctx, "project", "bucket", nil)

		t.Run("nil", func(t *ftt.Test) {
			ret, err := scheduleRequestFromTemplate(ctx, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ret, should.BeNil)
		})

		t.Run("empty", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{}))
			assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{}))
		})

		t.Run("not found", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			assert.Loosely(t, err, should.ErrLike("not found"))
			assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}))
			assert.Loosely(t, ret, should.BeNil)
		})

		t.Run("permission denied", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:unauthorized@example.com",
			})
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), should.BeNil)
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			assert.Loosely(t, err, should.ErrLike("not found"))
			assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}))
			assert.Loosely(t, ret, should.BeNil)
		})

		t.Run("canary", func(t *ftt.Test) {
			t.Run("false default", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
					Experiments: []string{
						"-" + bb.ExperimentBBCanarySoftware,
					},
				}), should.BeNil)

				t.Run("merge", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments:     map[string]bool{bb.ExperimentBBCanarySoftware: true},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments:     map[string]bool{bb.ExperimentBBCanarySoftware: true},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: true,
							bb.ExperimentNonProduction:    false,
						},
					}))
				})

				t.Run("ok", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
					}))
				})
			})

			t.Run("true default", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
					Experiments: []string{
						"+" + bb.ExperimentBBCanarySoftware,
					},
				}), should.BeNil)

				t.Run("merge", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
						},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
					}))
				})

				t.Run("ok", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: true,
							bb.ExperimentNonProduction:    false,
						},
					}))
				})
			})
		})

		t.Run("critical", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_YES,
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)

			t.Run("merge", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Critical:        pb.Trinary_NO,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Critical:        pb.Trinary_NO,
				}))
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_NO,
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
				}))
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}))
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Critical: pb.Trinary_YES,
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
				}))
			})
		})

		t.Run("exe", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Exe: &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)

			t.Run("merge", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe:             &pb.Executable{},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe:             &pb.Executable{},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Exe: &pb.Executable{},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
					}))
				})

				t.Run("non-empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "new",
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "new",
						},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Exe: &pb.Executable{
							CipdPackage: "package",
							CipdVersion: "new",
						},
					}))
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}))
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					Exe: &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
					},
				}))
			})
		})

		t.Run("gerrit changes", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 1,
							},
						},
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)

			t.Run("merge", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges:   []*pb.GerritChange{},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges:   []*pb.GerritChange{},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 1,
							},
						},
					}))
				})

				t.Run("non-empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 2,
							},
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 2,
							},
						},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						GerritChanges: []*pb.GerritChange{
							{
								Host:     "example.com",
								Project:  "project",
								Change:   1,
								Patchset: 2,
							},
						},
					}))
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}))
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "example.com",
							Project:  "project",
							Change:   1,
							Patchset: 1,
						},
					},
				}))
			})
		})

		t.Run("gitiles commit", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Input: &pb.Build_Input{
						GitilesCommit: &pb.GitilesCommit{
							Host:    "example.com",
							Project: "project",
							Ref:     "refs/heads/master",
						},
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}))
			assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Experiments: map[string]bool{
					bb.ExperimentBBCanarySoftware: false,
					bb.ExperimentNonProduction:    false,
				},
				GitilesCommit: &pb.GitilesCommit{
					Host:    "example.com",
					Project: "project",
					Ref:     "refs/heads/master",
				},
			}))
		})

		t.Run("input properties", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)

			t.Run("empty", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.BuildInputProperties{
					Build: datastore.MakeKey(ctx, "Build", 1),
				}), should.BeNil)

				t.Run("merge", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					}))
				})

				t.Run("ok", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
					}))
				})
			})

			t.Run("non-empty", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.BuildInputProperties{
					Build: datastore.MakeKey(ctx, "Build", 1),
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

				t.Run("merge", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						req := &pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties:      &structpb.Struct{},
						}
						ret, err := scheduleRequestFromTemplate(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties:      &structpb.Struct{},
						}))
						assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Experiments: map[string]bool{
								bb.ExperimentBBCanarySoftware: false,
								bb.ExperimentNonProduction:    false,
							},
							Properties: &structpb.Struct{},
						}))
					})

					t.Run("non-empty", func(t *ftt.Test) {
						req := &pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"other": {
										Kind: &structpb.Value_StringValue{
											StringValue: "other value",
										},
									},
								},
							},
						}
						ret, err := scheduleRequestFromTemplate(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
							TemplateBuildId: 1,
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"other": {
										Kind: &structpb.Value_StringValue{
											StringValue: "other value",
										},
									},
								},
							},
						}))
						assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Experiments: map[string]bool{
								bb.ExperimentBBCanarySoftware: false,
								bb.ExperimentNonProduction:    false,
							},
							Properties: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"other": {
										Kind: &structpb.Value_StringValue{
											StringValue: "other value",
										},
									},
								},
							},
						}))
					})
				})

				t.Run("ok", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					}))
				})
			})
		})

		t.Run("tags", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Tags: []string{
					"key:value",
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)

			t.Run("merge", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags:            []*pb.StringPair{},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags:            []*pb.StringPair{},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Tags: []*pb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
						},
					}))
				})

				t.Run("non-empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags: []*pb.StringPair{
							{
								Key:   "other",
								Value: "other",
							},
						},
					}
					ret, err := scheduleRequestFromTemplate(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
						TemplateBuildId: 1,
						Tags: []*pb.StringPair{
							{
								Key:   "other",
								Value: "other",
							},
						},
					}))
					assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Experiments: map[string]bool{
							bb.ExperimentBBCanarySoftware: false,
							bb.ExperimentNonProduction:    false,
						},
						Tags: []*pb.StringPair{
							{
								Key:   "other",
								Value: "other",
							},
						},
					}))
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}))
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
					Tags: []*pb.StringPair{
						{
							Key:   "key",
							Value: "value",
						},
					},
				}))
			})
		})

		t.Run("requested dimensions", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
						RequestedDimensions: []*pb.RequestedDimension{
							{Key: "key_in_db", Value: "value_in_db"},
						},
					},
				},
			}), should.BeNil)

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Dimensions: []*pb.RequestedDimension{
						{Key: "key_in_db", Value: "value_in_db"},
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
				}))
			})

			t.Run("override", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Dimensions: []*pb.RequestedDimension{
						{Key: "key_in_req", Value: "value_in_req"},
					},
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Dimensions: []*pb.RequestedDimension{
						{Key: "key_in_req", Value: "value_in_req"},
					},
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
				}))
			})
		})

		t.Run("priority", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						Priority: int32(30),
					},
				},
			}), should.BeNil)

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Priority: int32(30),
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
				}))
			})

			t.Run("override", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Priority:        int32(25),
				}
				ret, err := scheduleRequestFromTemplate(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Priority: int32(25),
					Experiments: map[string]bool{
						bb.ExperimentBBCanarySoftware: false,
						bb.ExperimentNonProduction:    false,
					},
				}))
			})
		})

		t.Run("ok", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				Experiments: []string{"-" + bb.ExperimentBBCanarySoftware},
			}), should.BeNil)
			req := &pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}
			ret, err := scheduleRequestFromTemplate(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req, should.Resemble(&pb.ScheduleBuildRequest{
				TemplateBuildId: 1,
			}))
			assert.Loosely(t, ret, should.Resemble(&pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Experiments: map[string]bool{
					bb.ExperimentBBCanarySoftware: false,
					bb.ExperimentNonProduction:    false,
				},
			}))
		})
	})

	ftt.Run("setDimensions", t, func(t *ftt.Test) {
		t.Run("config", func(t *ftt.Test) {
			t.Run("omit", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Dimensions: []string{
						"key:",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{}))
			})

			t.Run("simple", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Dimensions: []string{
						"key:value",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Key:   "key",
							Value: "value",
						},
					},
				}))
			})

			t.Run("expiration", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Dimensions: []string{
						"1:key:value",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Expiration: &durationpb.Duration{
								Seconds: 1,
							},
							Key:   "key",
							Value: "value",
						},
					},
				}))
			})

			t.Run("many", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Dimensions: []string{
						"key:",
						"key:value",
						"key:value:",
						"key:val:ue",
						"0:key:",
						"0:key:value",
						"0:key:value:",
						"0:key:val:ue",
						"1:key:",
						"1:key:value",
						"1:key:value:",
						"1:key:val:ue",
					},
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						TaskDimensions: []*pb.RequestedDimension{
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value:",
							},
							{
								Key:   "key",
								Value: "val:ue",
							},
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value:",
							},
							{
								Key:   "key",
								Value: "val:ue",
							},
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value",
							},
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value:",
							},
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "val:ue",
							},
						},
					},
				}))
			})

			t.Run("auto builder", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					AutoBuilderDimension: pb.Toggle_YES,
					Name:                 "builder",
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Key:   "builder",
							Value: "builder",
						},
					},
				}))
			})

			t.Run("builder > auto builder", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					AutoBuilderDimension: pb.Toggle_YES,
					Dimensions: []string{
						"1:builder:cfg builder",
					},
					Name: "auto builder",
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
					TaskDimensions: []*pb.RequestedDimension{
						{
							Expiration: &durationpb.Duration{
								Seconds: 1,
							},
							Key:   "builder",
							Value: "cfg builder",
						},
					},
				}))
			})

			t.Run("omit builder > auto builder", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					AutoBuilderDimension: pb.Toggle_YES,
					Dimensions: []string{
						"builder:",
					},
					Name: "auto builder",
				}
				b := &pb.Build{
					Infra: &pb.BuildInfra{
						Swarming: &pb.BuildInfra_Swarming{},
					},
				}

				setDimensions(nil, cfg, b, false)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{}))
			})
		})

		t.Run("request", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Dimensions: []*pb.RequestedDimension{
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "key",
						Value: "value",
					},
				},
			}
			b := &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			}

			setDimensions(req, nil, b, false)
			assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
				TaskDimensions: []*pb.RequestedDimension{
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "key",
						Value: "value",
					},
				},
			}))
		})

		t.Run("request > config", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Dimensions: []*pb.RequestedDimension{
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "req only",
						Value: "req value",
					},
					{
						Key:   "req only",
						Value: "req value",
					},
					{
						Key:   "key",
						Value: "req value",
					},
					{
						Key:   "key_to_exclude",
						Value: "",
					},
				},
			}
			cfg := &pb.BuilderConfig{
				AutoBuilderDimension: pb.Toggle_YES,
				Dimensions: []string{
					"1:cfg only:cfg value",
					"cfg only:cfg value",
					"cfg only:",
					"1:key:cfg value",
					"1:key_to_exclude:cfg value",
				},
				Name: "auto builder",
			}
			b := &pb.Build{
				Infra: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{},
				},
			}

			setDimensions(req, cfg, b, false)
			assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
				TaskDimensions: []*pb.RequestedDimension{
					{
						Key:   "builder",
						Value: "auto builder",
					},
					{
						Key:   "cfg only",
						Value: "cfg value",
					},
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "cfg only",
						Value: "cfg value",
					},
					{
						Key:   "key",
						Value: "req value",
					},
					{
						Key:   "req only",
						Value: "req value",
					},
					{
						Expiration: &durationpb.Duration{
							Seconds: 1,
						},
						Key:   "req only",
						Value: "req value",
					},
				},
			}))
		})
	})

	ftt.Run("setExecutable", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			b := &pb.Build{}

			setExecutable(nil, nil, b)
			assert.Loosely(t, b.Exe, should.Resemble(&pb.Executable{}))
		})

		t.Run("request only", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Exe: &pb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
					Cmd:         []string{"command"},
				},
			}
			b := &pb.Build{}

			setExecutable(req, nil, b)
			assert.Loosely(t, b.Exe, should.Resemble(&pb.Executable{
				CipdVersion: "version",
			}))
		})

		t.Run("config only", func(t *ftt.Test) {
			t.Run("exe", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Exe: &pb.Executable{
						CipdPackage: "package",
						CipdVersion: "version",
						Cmd:         []string{"command"},
					},
				}
				b := &pb.Build{}

				setExecutable(nil, cfg, b)
				assert.Loosely(t, b.Exe, should.Resemble(&pb.Executable{
					CipdPackage: "package",
					CipdVersion: "version",
					Cmd:         []string{"command"},
				}))
			})

			t.Run("recipe", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Exe: &pb.Executable{
						CipdPackage: "package 1",
						CipdVersion: "version 1",
						Cmd:         []string{"command"},
					},
					Recipe: &pb.BuilderConfig_Recipe{
						CipdPackage: "package 2",
						CipdVersion: "version 2",
					},
				}
				b := &pb.Build{}

				setExecutable(nil, cfg, b)
				assert.Loosely(t, b.Exe, should.Resemble(&pb.Executable{
					CipdPackage: "package 2",
					CipdVersion: "version 2",
					Cmd:         []string{"command"},
				}))
			})
		})

		t.Run("request > config", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Exe: &pb.Executable{
					CipdPackage: "package 1",
					CipdVersion: "version 1",
					Cmd:         []string{"command 1"},
				},
			}
			cfg := &pb.BuilderConfig{
				Exe: &pb.Executable{
					CipdPackage: "package 2",
					CipdVersion: "version 2",
					Cmd:         []string{"command 2"},
				},
			}
			b := &pb.Build{}

			setExecutable(req, cfg, b)
			assert.Loosely(t, b.Exe, should.Resemble(&pb.Executable{
				CipdPackage: "package 2",
				CipdVersion: "version 1",
				Cmd:         []string{"command 2"},
			}))
		})
	})

	ftt.Run("setExperiments", t, func(t *ftt.Test) {
		ctx := mathrand.Set(memory.Use(context.Background()), rand.New(rand.NewSource(1)))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")

		// settings.cfg
		gCfg := &pb.SettingsCfg{
			Experiment: &pb.ExperimentSettings{},
		}

		// builder config
		cfg := &pb.BuilderConfig{
			Experiments: map[string]int32{},
		}

		// base datastore entity (and embedded Build Proto)
		ent := &model.Build{
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Exe: &pb.Executable{},
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
				Input: &pb.Build_Input{},
			},
		}

		expect := &model.Build{
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Exe: &pb.Executable{
					Cmd: []string{"recipes"},
				},
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
				Input: &pb.Build_Input{},
			},
		}

		req := &pb.ScheduleBuildRequest{
			Experiments: map[string]bool{},
		}

		setExps := func() {
			normalizeSchedule(req)
			setExperiments(ctx, req, cfg, gCfg, ent.Proto)
			setExperimentsFromProto(ent)
		}
		initReasons := func() map[string]pb.BuildInfra_Buildbucket_ExperimentReason {
			er := make(map[string]pb.BuildInfra_Buildbucket_ExperimentReason)
			expect.Proto.Infra.Buildbucket.ExperimentReasons = er
			return er
		}

		t.Run("nil", func(t *ftt.Test) {
			setExps()
			assert.Loosely(t, ent, should.Resemble(expect))
		})

		t.Run("dice rolling works", func(t *ftt.Test) {
			for i := 0; i < 100; i += 10 {
				cfg.Experiments["exp"+strconv.Itoa(i)] = int32(i)
			}
			setExps()

			assert.Loosely(t, ent.Proto.Input.Experiments, should.Resemble([]string{
				"exp60", "exp70", "exp80", "exp90",
			}))
		})

		t.Run("command", func(t *ftt.Test) {
			t.Run("recipes", func(t *ftt.Test) {
				req.Experiments[bb.ExperimentBBAgent] = false
				setExps()

				assert.Loosely(t, ent.Proto.Exe, should.Resemble(&pb.Executable{
					Cmd: []string{"recipes"},
				}))
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons[bb.ExperimentBBAgent],
					should.Equal(pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED))
			})

			t.Run("recipes (explicit)", func(t *ftt.Test) {
				ent.Proto.Exe.Cmd = []string{"recipes"}
				req.Experiments[bb.ExperimentBBAgent] = false
				setExps()

				assert.Loosely(t, ent.Proto.Exe, should.Resemble(&pb.Executable{
					Cmd: []string{"recipes"},
				}))
				assert.Loosely(t, ent.Proto.Input.Experiments, should.BeEmpty)
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons[bb.ExperimentBBAgent],
					should.Equal(pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG))
			})

			t.Run("luciexe (experiment)", func(t *ftt.Test) {
				req.Experiments[bb.ExperimentBBAgent] = true
				setExps()

				assert.Loosely(t, ent.Proto.Exe, should.Resemble(&pb.Executable{
					Cmd: []string{"luciexe"},
				}))
				assert.Loosely(t, ent.Proto.Input.Experiments, should.Contain(bb.ExperimentBBAgent))
				assert.Loosely(t, ent.Experiments, should.Contain("+"+bb.ExperimentBBAgent))
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons[bb.ExperimentBBAgent],
					should.Equal(pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED))
			})

			t.Run("luciexe (explicit)", func(t *ftt.Test) {
				ent.Proto.Exe.Cmd = []string{"luciexe"}
				setExps()

				assert.Loosely(t, ent.Proto.Exe, should.Resemble(&pb.Executable{
					Cmd: []string{"luciexe"},
				}))
				assert.Loosely(t, ent.Proto.Input.Experiments, should.Contain(bb.ExperimentBBAgent))
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons[bb.ExperimentBBAgent],
					should.Equal(pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG))
			})

			t.Run("cmd > experiment", func(t *ftt.Test) {
				req.Experiments[bb.ExperimentBBAgent] = false
				ent.Proto.Exe.Cmd = []string{"command"}
				setExps()

				assert.Loosely(t, ent.Proto.Exe, should.Resemble(&pb.Executable{
					Cmd: []string{"command"},
				}))
				assert.Loosely(t, ent.Proto.Input.Experiments, should.Contain(bb.ExperimentBBAgent))
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons[bb.ExperimentBBAgent],
					should.Equal(pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG))
			})
		})

		t.Run("request only", func(t *ftt.Test) {
			req.Experiments["experiment1"] = true
			req.Experiments["experiment2"] = false
			setExps()

			expect.Experiments = []string{
				"+experiment1",
				"-experiment2",
			}
			expect.Proto.Input.Experiments = []string{"experiment1"}
			er := initReasons()
			er["experiment1"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
			er["experiment2"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED

			assert.Loosely(t, ent, should.Resemble(expect))
		})

		t.Run("legacy only", func(t *ftt.Test) {
			req.Canary = pb.Trinary_YES
			req.Experimental = pb.Trinary_NO
			setExps()

			expect.Canary = true
			expect.Experiments = []string{
				"+" + bb.ExperimentBBCanarySoftware,
				"-" + bb.ExperimentNonProduction,
			}
			expect.Proto.Canary = true
			expect.Proto.Input.Experiments = []string{bb.ExperimentBBCanarySoftware}
			er := initReasons()
			er[bb.ExperimentBBCanarySoftware] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
			er[bb.ExperimentNonProduction] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED

			assert.Loosely(t, ent, should.Resemble(expect))
		})

		t.Run("config only", func(t *ftt.Test) {
			cfg.Experiments["experiment1"] = 100
			cfg.Experiments["experiment2"] = 0
			setExps()

			expect.Experiments = []string{
				"+experiment1",
				"-experiment2",
			}
			expect.Proto.Input.Experiments = []string{"experiment1"}
			er := initReasons()
			er["experiment1"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG
			er["experiment2"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_BUILDER_CONFIG

			assert.Loosely(t, ent, should.Resemble(expect))
		})

		t.Run("override", func(t *ftt.Test) {
			t.Run("request > legacy", func(t *ftt.Test) {
				req.Canary = pb.Trinary_YES
				req.Experimental = pb.Trinary_NO
				req.Experiments[bb.ExperimentBBCanarySoftware] = false
				req.Experiments[bb.ExperimentNonProduction] = true
				setExps()

				expect.Experiments = []string{
					"+" + bb.ExperimentNonProduction,
					"-" + bb.ExperimentBBCanarySoftware,
				}
				expect.Experimental = true
				expect.Proto.Input.Experimental = true
				expect.Proto.Input.Experiments = []string{bb.ExperimentNonProduction}
				er := initReasons()
				er[bb.ExperimentNonProduction] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
				er[bb.ExperimentBBCanarySoftware] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED

				assert.Loosely(t, ent, should.Resemble(expect))
			})

			t.Run("legacy > config", func(t *ftt.Test) {
				req.Canary = pb.Trinary_YES
				req.Experimental = pb.Trinary_NO
				cfg.Experiments[bb.ExperimentBBCanarySoftware] = 0
				cfg.Experiments[bb.ExperimentNonProduction] = 100
				setExps()

				expect.Experiments = []string{
					"+" + bb.ExperimentBBCanarySoftware,
					"-" + bb.ExperimentNonProduction,
				}
				expect.Canary = true
				expect.Proto.Canary = true
				expect.Proto.Input.Experiments = []string{bb.ExperimentBBCanarySoftware}
				er := initReasons()
				er[bb.ExperimentNonProduction] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
				er[bb.ExperimentBBCanarySoftware] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED

				assert.Loosely(t, ent, should.Resemble(expect))
			})

			t.Run("request > config", func(t *ftt.Test) {
				req.Experiments["experiment1"] = true
				req.Experiments["experiment2"] = false
				cfg.Experiments["experiment1"] = 0
				cfg.Experiments["experiment2"] = 100
				setExps()

				expect.Experiments = []string{
					"+experiment1",
					"-experiment2",
				}
				expect.Proto.Input.Experiments = []string{"experiment1"}
				er := initReasons()
				er["experiment1"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
				er["experiment2"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED

				assert.Loosely(t, ent, should.Resemble(expect))
			})

			t.Run("request > legacy > config", func(t *ftt.Test) {
				req.Canary = pb.Trinary_YES
				req.Experimental = pb.Trinary_NO
				req.Experiments[bb.ExperimentBBCanarySoftware] = false
				req.Experiments[bb.ExperimentNonProduction] = true
				req.Experiments["experiment1"] = true
				req.Experiments["experiment2"] = false
				cfg.Experiments[bb.ExperimentBBCanarySoftware] = 100
				cfg.Experiments[bb.ExperimentNonProduction] = 100
				cfg.Experiments["experiment1"] = 0
				cfg.Experiments["experiment2"] = 0
				setExps()

				expect.Experiments = []string{
					"+experiment1",
					"+" + bb.ExperimentNonProduction,
					"-experiment2",
					"-" + bb.ExperimentBBCanarySoftware,
				}
				expect.Experimental = true
				expect.Proto.Input.Experimental = true
				expect.Proto.Input.Experiments = []string{
					"experiment1",
					bb.ExperimentNonProduction,
				}
				er := initReasons()
				er["experiment1"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
				er["experiment2"] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
				er[bb.ExperimentBBCanarySoftware] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
				er[bb.ExperimentNonProduction] = pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED

				assert.Loosely(t, ent, should.Resemble(expect))
			})
		})

		t.Run("global configuration", func(t *ftt.Test) {
			addExp := func(name string, dflt, min int32, inactive bool, b *pb.BuilderPredicate) {
				gCfg.Experiment.Experiments = append(gCfg.Experiment.Experiments, &pb.ExperimentSettings_Experiment{
					Name:         name,
					DefaultValue: dflt,
					MinimumValue: min,
					Builders:     b,
					Inactive:     inactive,
				})
			}

			t.Run("default always", func(t *ftt.Test) {
				addExp("always", 100, 0, false, nil)

				t.Run("will fill in if unset", func(t *ftt.Test) {
					setExps()

					assert.Loosely(t, ent.Proto.Input.Experiments, should.Resemble([]string{"always"}))
					assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons, should.Resemble(map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
						"always": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_DEFAULT,
					}))
				})

				t.Run("can be overridden from request", func(t *ftt.Test) {
					req.Experiments["always"] = false
					setExps()

					assert.Loosely(t, ent.Proto.Input.Experiments, should.BeEmpty)
					assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons, should.Resemble(map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
						"always": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
					}))
				})
			})

			t.Run("per builder", func(t *ftt.Test) {
				addExp("per.builder", 100, 0, false, &pb.BuilderPredicate{
					Regex: []string{"project/bucket/builder"},
				})
				addExp("other.builder", 100, 0, false, &pb.BuilderPredicate{
					Regex: []string{"project/bucket/other"},
				})
				setExps()

				assert.Loosely(t, ent.Proto.Input.Experiments, should.Resemble([]string{"per.builder"}))
				assert.Loosely(t, ent.Experiments, should.Contain("-other.builder"))
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons, should.Resemble(map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
					"per.builder":   pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_DEFAULT,
					"other.builder": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_DEFAULT,
				}))
			})

			t.Run("min value", func(t *ftt.Test) {
				// note that default == 0, min == 100 is a bit silly, but works for this
				// test.
				addExp("min.value", 0, 100, false, nil)

				t.Run("overrides builder config", func(t *ftt.Test) {
					cfg.Experiments["min.value"] = 0
					setExps()

					assert.Loosely(t, ent.Proto.Input.Experiments, should.Resemble([]string{"min.value"}))
					assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons, should.Resemble(map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
						"min.value": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_MINIMUM,
					}))
				})

				t.Run("can be overridden from request", func(t *ftt.Test) {
					req.Experiments["min.value"] = false
					setExps()

					assert.Loosely(t, ent.Proto.Input.Experiments, should.BeEmpty)
					assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons, should.Resemble(map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
						"min.value": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
					}))
				})
			})

			t.Run("inactive", func(t *ftt.Test) {
				addExp("inactive", 30, 30, true, nil)
				addExp("other_inactive", 30, 30, true, nil)
				cfg.Experiments["inactive"] = 100
				setExps()

				assert.Loosely(t, ent.Proto.Input.Experiments, should.BeEmpty)
				assert.Loosely(t, ent.Proto.Infra.Buildbucket.ExperimentReasons, should.Resemble(map[string]pb.BuildInfra_Buildbucket_ExperimentReason{
					"inactive": pb.BuildInfra_Buildbucket_EXPERIMENT_REASON_GLOBAL_INACTIVE,
					// Note that other_inactive wasn't requested in the build so it's
					// absent here.
				}))
			})
		})
	})

	ftt.Run("buildFromScheduleRequest", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		t.Run("backend is enabled", func(t *ftt.Test) {
			s := &pb.SettingsCfg{
				Backends: []*pb.BackendSetting{
					{
						Target:   "swarming://chromium-swarm",
						Hostname: "chromium-swarm.appspot.com",
					},
				},
				Cipd: &pb.CipdSettings{
					Server: "cipd_server",
				},
				Swarming: &pb.SwarmingSettings{
					BbagentPackage: &pb.SwarmingSettings_Package{
						PackageName: "cipd_pkg/${platform}",
						Version:     "cipd_vers",
					},
				},
			}
			bldrCfg := &pb.BuilderConfig{
				Dimensions: []string{
					"key:value",
				},
				ServiceAccount: "account",
				Backend: &pb.BuilderConfig_Backend{
					Target: "swarming://chromium-swarm",
				},
				Experiments: map[string]int32{
					bb.ExperimentBackendAlt: 100,
				},
				WaitForCapacity: pb.Trinary_YES,
			}
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Bucket:  "bucket",
					Builder: "builder",
					Project: "project",
				},
				RequestId: "request_id",
				Priority:  100,
			}

			buildResult := buildFromScheduleRequest(ctx, req, nil, "", bldrCfg, s)
			expectedBackendConfig := &structpb.Struct{}
			expectedBackendConfig.Fields = make(map[string]*structpb.Value)
			expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 100}}
			expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}
			expectedBackendConfig.Fields["agent_binary_cipd_pkg"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "cipd_pkg/${platform}"}}
			expectedBackendConfig.Fields["agent_binary_cipd_vers"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "cipd_vers"}}
			expectedBackendConfig.Fields["agent_binary_cipd_server"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "https://cipd_server"}}
			expectedBackendConfig.Fields["agent_binary_cipd_filename"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "bbagent${EXECUTABLE_SUFFIX}"}}
			expectedBackendConfig.Fields["wait_for_capacity"] = &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: true}}

			assert.Loosely(t, buildResult.Infra.Backend, should.Match(&pb.BuildInfra_Backend{
				Caches: []*pb.CacheEntry{
					{
						Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
						Path:             "cache/builder",
						WaitForWarmCache: &durationpb.Duration{Seconds: 240},
					},
				},
				Config:   expectedBackendConfig,
				Hostname: "chromium-swarm.appspot.com",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Target: "swarming://chromium-swarm",
					},
				},
				TaskDimensions: []*pb.RequestedDimension{
					{
						Key:   "key",
						Value: "value",
					},
				},
			}))
		})
	})

	ftt.Run("setInfra", t, func(t *ftt.Test) {
		ctx := mathrand.Set(memory.Use(context.Background()), rand.New(rand.NewSource(1)))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		t.Run("nil", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}

			setInfra(ctx, nil, nil, b, nil)
			assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "app.appspot.com",
				},
				Logdog: &pb.BuildInfra_LogDog{
					Project: "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
			}))
		})

		t.Run("bbagent", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			s := &pb.SettingsCfg{
				KnownPublicGerritHosts: []string{
					"host",
				},
			}

			setInfra(ctx, nil, nil, b, s)
			assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "app.appspot.com",
					KnownPublicGerritHosts: []string{
						"host",
					},
				},
				Logdog: &pb.BuildInfra_LogDog{
					Project: "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
			}))
		})

		t.Run("logdog", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			s := &pb.SettingsCfg{
				Logdog: &pb.LogDogSettings{
					Hostname: "host",
				},
			}

			setInfra(ctx, nil, nil, b, s)
			assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "app.appspot.com",
				},
				Logdog: &pb.BuildInfra_LogDog{
					Hostname: "host",
					Project:  "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
			}))
		})

		t.Run("resultdb", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Id: 1,
			}
			s := &pb.SettingsCfg{
				Resultdb: &pb.ResultDBSettings{
					Hostname: "host",
				},
			}

			bqExports := []*rdbPb.BigQueryExport{}
			cfg := &pb.BuilderConfig{
				Resultdb: &pb.BuilderConfig_ResultDB{
					Enable:    true,
					BqExports: bqExports,
				},
			}

			setInfra(ctx, nil, cfg, b, s)
			assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					PayloadPath: "kitchen-checkout",
					CacheDir:    "cache",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "app.appspot.com",
				},
				Logdog: &pb.BuildInfra_LogDog{
					Hostname: "",
					Project:  "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{
					Hostname:  "host",
					Enable:    true,
					BqExports: bqExports,
				},
			}))
		})

		t.Run("config", func(t *ftt.Test) {
			t.Run("recipe", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Recipe: &pb.BuilderConfig_Recipe{
						CipdPackage: "package",
						Name:        "name",
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(ctx, nil, cfg, b, nil)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Recipe: &pb.BuildInfra_Recipe{
						CipdPackage: "package",
						Name:        "name",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
				}))
			})
		})

		t.Run("request", func(t *ftt.Test) {
			t.Run("dimensions", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{
							Expiration: &durationpb.Duration{
								Seconds: 1,
							},
							Key:   "key",
							Value: "value",
						},
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(ctx, req, nil, b, nil)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
						RequestedDimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Seconds: 1,
								},
								Key:   "key",
								Value: "value",
							},
						},
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
				}))
			})

			t.Run("properties", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInfra(ctx, req, nil, b, nil)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
						RequestedProperties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
				}))
			})
		})
	})

	ftt.Run("setSwarmingOrBackend", t, func(t *ftt.Test) {
		ctx := mathrand.Set(memory.Use(context.Background()), rand.New(rand.NewSource(1)))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		t.Run("nil", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Hostname: "host",
						Project:  "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
				},
			}

			setSwarmingOrBackend(ctx, nil, nil, b, nil)
			assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "app.appspot.com",
				},
				Logdog: &pb.BuildInfra_LogDog{
					Hostname: "host",
					Project:  "project",
				},
				Resultdb: &pb.BuildInfra_ResultDB{},
				Swarming: &pb.BuildInfra_Swarming{
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{
							Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path: "builder",
							WaitForWarmCache: &durationpb.Duration{
								Seconds: 240,
							},
						},
					},
					Priority: 30,
				},
			}))
		})
		t.Run("priority", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Hostname: "host",
						Project:  "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
				},
				Input: &pb.Build_Input{
					Experiments: []string{},
				},
			}
			t.Run("default production", func(t *ftt.Test) {
				setSwarmingOrBackend(ctx, nil, nil, b, nil)
				assert.Loosely(t, b.Infra.Swarming.Priority, should.Equal(30))
				assert.Loosely(t, b.Input.Experimental, should.BeFalse)
			})

			t.Run("non-production", func(t *ftt.Test) {
				b.Input.Experiments = append(b.Input.Experiments, bb.ExperimentNonProduction)
				setSwarmingOrBackend(ctx, nil, nil, b, nil)
				assert.Loosely(t, b.Infra.Swarming.Priority, should.Equal(255))
				assert.Loosely(t, b.Input.Experimental, should.BeFalse)
			})

			t.Run("req > experiment", func(t *ftt.Test) {
				b.Input.Experiments = append(b.Input.Experiments, bb.ExperimentNonProduction)
				req := &pb.ScheduleBuildRequest{
					Priority: 1,
				}
				setSwarmingOrBackend(ctx, req, nil, b, nil)
				assert.Loosely(t, b.Infra.Swarming.Priority, should.Equal(1))
			})
		})

		t.Run("swarming", func(t *ftt.Test) {
			t.Run("no dimensions", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Priority:       1,
					ServiceAccount: "account",
					SwarmingHost:   "host",
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							PayloadPath: "kitchen-checkout",
							CacheDir:    "cache",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
					},
				}

				setSwarmingOrBackend(ctx, nil, cfg, b, nil)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						PayloadPath: "kitchen-checkout",
						CacheDir:    "cache",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
					Swarming: &pb.BuildInfra_Swarming{
						Caches: []*pb.BuildInfra_Swarming_CacheEntry{
							{
								Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
								Path: "builder",
								WaitForWarmCache: &durationpb.Duration{
									Seconds: 240,
								},
							},
						},
						Hostname:           "host",
						Priority:           1,
						TaskServiceAccount: "account",
					},
				}))
			})

			t.Run("caches", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								PayloadPath: "kitchen-checkout",
								CacheDir:    "cache",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "app.appspot.com",
							},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
						},
					}

					setSwarmingOrBackend(ctx, nil, nil, b, nil)
					assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
							},
							Priority: 30,
						},
					}))
				})

				t.Run("global", func(t *ftt.Test) {
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								PayloadPath: "kitchen-checkout",
								CacheDir:    "cache",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "app.appspot.com",
							},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
						},
					}
					s := &pb.SettingsCfg{
						Swarming: &pb.SwarmingSettings{
							GlobalCaches: []*pb.BuilderConfig_CacheEntry{
								{
									Path: "cache",
								},
							},
						},
					}

					setSwarmingOrBackend(ctx, nil, nil, b, s)
					assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
								{
									Name: "cache",
									Path: "cache",
								},
							},
							Priority: 30,
						},
					}))
				})

				t.Run("config", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Caches: []*pb.BuilderConfig_CacheEntry{
							{
								Path: "cache",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								PayloadPath: "kitchen-checkout",
								CacheDir:    "cache",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "app.appspot.com",
							},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
						},
					}

					setSwarmingOrBackend(ctx, nil, cfg, b, nil)
					assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
								{
									Name: "cache",
									Path: "cache",
								},
							},
							Priority: 30,
						},
					}))
				})

				t.Run("config > global", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Caches: []*pb.BuilderConfig_CacheEntry{
							{
								Name: "builder only name",
								Path: "builder only path",
							},
							{
								Name: "name",
								Path: "builder path",
							},
							{
								Name: "builder name",
								Path: "path",
							},
							{
								EnvVar: "builder env",
								Path:   "env",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								PayloadPath: "kitchen-checkout",
								CacheDir:    "cache",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "app.appspot.com",
							},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{},
						},
					}
					s := &pb.SettingsCfg{
						Swarming: &pb.SwarmingSettings{
							GlobalCaches: []*pb.BuilderConfig_CacheEntry{
								{
									Name: "global only name",
									Path: "global only path",
								},
								{
									Name: "name",
									Path: "global path",
								},
								{
									Name: "global name",
									Path: "path",
								},
								{
									EnvVar: "global env",
									Path:   "path",
								},
							},
						},
					}

					setSwarmingOrBackend(ctx, nil, cfg, b, s)
					assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							CacheDir:    "cache",
							PayloadPath: "kitchen-checkout",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
						Swarming: &pb.BuildInfra_Swarming{
							Caches: []*pb.BuildInfra_Swarming_CacheEntry{
								{
									Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
									Path: "builder",
									WaitForWarmCache: &durationpb.Duration{
										Seconds: 240,
									},
								},
								{
									Name: "builder only name",
									Path: "builder only path",
								},
								{
									Name: "name",
									Path: "builder path",
								},
								{
									EnvVar: "builder env",
									Name:   "env",
									Path:   "env",
								},
								{
									Name: "global only name",
									Path: "global only path",
								},
								{
									Name: "builder name",
									Path: "path",
								},
							},
							Priority: 30,
						},
					}))
				})
			})

			t.Run("parent run id", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Swarming: &pb.ScheduleBuildRequest_Swarming{
						ParentRunId: "id",
					},
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							PayloadPath: "kitchen-checkout",
							CacheDir:    "cache",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
					},
				}

				setSwarmingOrBackend(ctx, req, nil, b, nil)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
					Swarming: &pb.BuildInfra_Swarming{
						Caches: []*pb.BuildInfra_Swarming_CacheEntry{
							{
								Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
								Path: "builder",
								WaitForWarmCache: &durationpb.Duration{
									Seconds: 240,
								},
							},
						},
						ParentRunId: "id",
						Priority:    30,
					},
				}))
			})

			t.Run("priority", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Priority: 1,
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &pb.BuildInfra{
						Bbagent: &pb.BuildInfra_BBAgent{
							PayloadPath: "kitchen-checkout",
							CacheDir:    "cache",
						},
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
						Logdog: &pb.BuildInfra_LogDog{
							Project: "project",
						},
						Resultdb: &pb.BuildInfra_ResultDB{},
					},
				}

				setSwarmingOrBackend(ctx, req, nil, b, nil)
				assert.Loosely(t, b.Infra, should.Resemble(&pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Project: "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
					Swarming: &pb.BuildInfra_Swarming{
						Caches: []*pb.BuildInfra_Swarming_CacheEntry{
							{
								Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
								Path: "builder",
								WaitForWarmCache: &durationpb.Duration{
									Seconds: 240,
								},
							},
						},
						Priority: 1,
					},
				}))
			})
		})

		t.Run("backend", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Infra: &pb.BuildInfra{
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir:    "cache",
						PayloadPath: "kitchen-checkout",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
					Logdog: &pb.BuildInfra_LogDog{
						Hostname: "host",
						Project:  "project",
					},
					Resultdb: &pb.BuildInfra_ResultDB{},
				},
			}
			s := &pb.SettingsCfg{
				Backends: []*pb.BackendSetting{
					{
						Target:   "swarming://chromium-swarm",
						Hostname: "chromium-swarm.appspot.com",
					},
				},
			}
			bldrCfg := &pb.BuilderConfig{
				ServiceAccount: "account",
				Priority:       200,
				Backend: &pb.BuilderConfig_Backend{
					Target: "swarming://chromium-swarm",
				},
				Experiments: map[string]int32{
					bb.ExperimentBackendAlt: 100,
				},
			}

			// Need these to be set so that setSwarmingOrBackend can be set.
			setExecutable(nil, bldrCfg, b)
			setInput(ctx, nil, bldrCfg, b)
			setExperiments(ctx, nil, bldrCfg, s, b)

			t.Run("use builder Priority and ServiceAccount", func(t *ftt.Test) {
				setSwarmingOrBackend(ctx, nil, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 200}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}

				assert.Loosely(t, b.Infra.Backend, should.Match(&pb.BuildInfra_Backend{
					Caches: []*pb.CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "cache/builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
					Config:   expectedBackendConfig,
					Hostname: "chromium-swarm.appspot.com",
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
					},
				}))
			})

			t.Run("use backend priority and ServiceAccount", func(t *ftt.Test) {
				bldrCfg.Backend.ConfigJson = "{\"priority\": 2, \"service_account\": \"service_account\"}"
				setSwarmingOrBackend(ctx, nil, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 2}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "service_account"}}

				assert.Loosely(t, b.Infra.Backend, should.Match(&pb.BuildInfra_Backend{
					Caches: []*pb.CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "cache/builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
					Config:   expectedBackendConfig,
					Hostname: "chromium-swarm.appspot.com",
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
					},
				}))
			})

			t.Run("use user requested priority", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{Priority: 22}
				setSwarmingOrBackend(ctx, req, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 22}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}

				assert.Loosely(t, b.Infra.Backend, should.Match(&pb.BuildInfra_Backend{
					Caches: []*pb.CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "cache/builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
					Config:   expectedBackendConfig,
					Hostname: "chromium-swarm.appspot.com",
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
					},
				}))
			})

			t.Run("backend alt is used", func(t *ftt.Test) {
				bldrCfg.BackendAlt = &pb.BuilderConfig_Backend{
					Target: "swarming://chromium-swarm-alt",
				}

				setSwarmingOrBackend(ctx, nil, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 200}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}

				assert.Loosely(t, b.Infra.Backend, should.Match(&pb.BuildInfra_Backend{
					Caches: []*pb.CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "cache/builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
					Config: expectedBackendConfig,
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm-alt",
						},
					},
				}))
			})

			t.Run("backend_alt exp is true, derive backend from swarming", func(t *ftt.Test) {
				bldrCfg := &pb.BuilderConfig{
					ServiceAccount: "account",
					Priority:       200,
					Experiments: map[string]int32{
						bb.ExperimentBackendAlt: 100,
					},
					SwarmingHost: "chromium-swarming.appspot.com",
				}

				s.SwarmingBackends = map[string]string{
					"chromium-swarming.appspot.com": "swarming://chromium-swarm",
				}
				setSwarmingOrBackend(ctx, nil, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 200}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}

				assert.Loosely(t, b.Infra.Swarming, should.BeNil)
				assert.Loosely(t, b.Infra.Backend, should.Match(&pb.BuildInfra_Backend{
					Caches: []*pb.CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "cache/builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
					Config:   expectedBackendConfig,
					Hostname: "chromium-swarm.appspot.com",
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
					},
				}))
			})

			t.Run("backend_alt exp is true but no swarming to backend mapping, so use swarming", func(t *ftt.Test) {
				bldrCfg := &pb.BuilderConfig{
					ServiceAccount: "account",
					Priority:       200,
					Experiments: map[string]int32{
						bb.ExperimentBackendAlt: 100,
					},
				}

				setSwarmingOrBackend(ctx, nil, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 200}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}

				assert.Loosely(t, b.Infra.Backend, should.BeNil)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
					TaskServiceAccount: "account",
					Priority:           200,
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
				}))
			})

			t.Run("swarming is used", func(t *ftt.Test) {
				bldrCfg := &pb.BuilderConfig{
					ServiceAccount: "account",
					Priority:       200,
				}

				setSwarmingOrBackend(ctx, nil, bldrCfg, b, s)

				expectedBackendConfig := &structpb.Struct{}
				expectedBackendConfig.Fields = make(map[string]*structpb.Value)
				expectedBackendConfig.Fields["priority"] = &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 200}}
				expectedBackendConfig.Fields["service_account"] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}}

				assert.Loosely(t, b.Infra.Backend, should.BeNil)
				assert.Loosely(t, b.Infra.Swarming, should.Resemble(&pb.BuildInfra_Swarming{
					TaskServiceAccount: "account",
					Priority:           200,
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{
							Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path:             "builder",
							WaitForWarmCache: &durationpb.Duration{Seconds: 240},
						},
					},
				}))
			})
		})
	})

	ftt.Run("setInput", t, func(t *ftt.Test) {
		ctx := memlogger.Use(context.Background())

		t.Run("nil", func(t *ftt.Test) {
			b := &pb.Build{}

			setInput(ctx, nil, nil, b)
			assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
				Properties: &structpb.Struct{},
			}))
		})

		t.Run("request", func(t *ftt.Test) {
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{}
					b := &pb.Build{}

					setInput(ctx, req, nil, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{},
					}))
				})

				t.Run("non-empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"int": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"str": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					}
					b := &pb.Build{}

					setInput(ctx, req, nil, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"int": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"str": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					}))
				})
			})
		})

		t.Run("config", func(t *ftt.Test) {
			t.Run("properties", func(t *ftt.Test) {
				cfg := &pb.BuilderConfig{
					Properties: "{\"int\": 1, \"str\": \"value\"}",
				}
				b := &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				setInput(ctx, nil, cfg, b)
				assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"int": {
								Kind: &structpb.Value_NumberValue{
									NumberValue: 1,
								},
							},
							"str": {
								Kind: &structpb.Value_StringValue{
									StringValue: "value",
								},
							},
						},
					},
				}))
			})

			t.Run("recipe", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{},
								},
							},
						},
					}))
				})

				t.Run("properties", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{
							Properties: []string{
								"key:value",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "",
									},
								},
							},
						},
					}))
				})

				t.Run("properties json", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{
							PropertiesJ: []string{
								"str:\"value\"",
								"int:1",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"int": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"recipe": {
									Kind: &structpb.Value_StringValue{},
								},
								"str": {
									Kind: &structpb.Value_StringValue{
										StringValue: "value",
									},
								},
							},
						},
					}))
				})

				t.Run("recipe", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{
							Name: "recipe",
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "recipe",
									},
								},
							},
						},
					}))
				})

				t.Run("properties json > properties", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{
							Properties: []string{
								"key:value",
							},
							PropertiesJ: []string{
								"key:1",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": {
									Kind: &structpb.Value_NumberValue{
										NumberValue: 1,
									},
								},
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "",
									},
								},
							},
						},
					}))
				})

				t.Run("recipe > properties", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{
							Name: "recipe",
							Properties: []string{
								"recipe:value",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "recipe",
									},
								},
							},
						},
					}))
				})

				t.Run("recipe > properties json", func(t *ftt.Test) {
					cfg := &pb.BuilderConfig{
						Recipe: &pb.BuilderConfig_Recipe{
							Name: "recipe",
							PropertiesJ: []string{
								"recipe:\"value\"",
							},
						},
					}
					b := &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					setInput(ctx, nil, cfg, b)
					assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"recipe": {
									Kind: &structpb.Value_StringValue{
										StringValue: "recipe",
									},
								},
							},
						},
					}))
				})
			})
		})

		t.Run("request > config", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"allowed": {
							Kind: &structpb.Value_StringValue{
								StringValue: "I'm alright",
							},
						},
						"override": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
						"req key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
					},
				},
			}
			cfg := &pb.BuilderConfig{
				Properties:               "{\"override\": \"cfg value\", \"allowed\": \"stuff\", \"cfg key\": \"cfg value\"}",
				AllowedPropertyOverrides: []string{"allowed"},
			}
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}

			setInput(ctx, req, cfg, b)
			assert.Loosely(t, b.Input, should.Resemble(&pb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"allowed": {
							Kind: &structpb.Value_StringValue{
								StringValue: "I'm alright",
							},
						},
						"cfg key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "cfg value",
							},
						},
						"override": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
						"req key": {
							Kind: &structpb.Value_StringValue{
								StringValue: "req value",
							},
						},
					},
				},
			}))
			assert.Loosely(t, ctx, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, "ScheduleBuild: Unpermitted Override for property \"override\""))
			assert.Loosely(t, ctx, convey.Adapt(memlogger.ShouldNotHaveLog)(logging.Warning, "ScheduleBuild: Unpermitted Override for property \"allowed\""))
			assert.Loosely(t, ctx, convey.Adapt(memlogger.ShouldNotHaveLog)(logging.Warning, "ScheduleBuild: Unpermitted Override for property \"cfg key\""))
			assert.Loosely(t, ctx, convey.Adapt(memlogger.ShouldNotHaveLog)(logging.Warning, "ScheduleBuild: Unpermitted Override for property \"req key\""))
		})
	})

	ftt.Run("setTags", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			b := &pb.Build{}

			setTags(nil, b, "")
			assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{}))
		})

		t.Run("request", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Tags: []*pb.StringPair{
					{
						Key:   "key2",
						Value: "value2",
					},
					{
						Key:   "key1",
						Value: "value1",
					},
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b, "")
			assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
				{
					Key:   "key1",
					Value: "value1",
				},
				{
					Key:   "key2",
					Value: "value2",
				},
			}))
		})

		t.Run("builder", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b, "")
			assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
				{
					Key:   "builder",
					Value: "builder",
				},
			}))
		})

		t.Run("gitiles commit", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "id",
					Ref:     "ref",
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b, "")
			assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
				{
					Key:   "buildset",
					Value: "commit/gitiles/host/project/+/id",
				},
				{
					Key:   "gitiles_ref",
					Value: "ref",
				},
			}))
		})

		t.Run("partial gitiles commit", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Ref:     "ref",
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b, "")
			assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
				{
					Key:   "gitiles_ref",
					Value: "ref",
				},
			}))
		})

		t.Run("gerrit changes", func(t *ftt.Test) {
			t.Run("one", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "host",
							Change:   1,
							Patchset: 2,
						},
					},
				}
				normalizeSchedule(req)
				b := &pb.Build{}

				setTags(req, b, "")
				assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
					{
						Key:   "buildset",
						Value: "patch/gerrit/host/1/2",
					},
				}))
			})

			t.Run("many", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "host",
							Change:   3,
							Patchset: 4,
						},
						{
							Host:     "host",
							Change:   1,
							Patchset: 2,
						},
					},
				}
				normalizeSchedule(req)
				b := &pb.Build{}

				setTags(req, b, "")
				assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
					{
						Key:   "buildset",
						Value: "patch/gerrit/host/1/2",
					},
					{
						Key:   "buildset",
						Value: "patch/gerrit/host/3/4",
					},
				}))
			})
		})

		t.Run("various", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				GerritChanges: []*pb.GerritChange{
					{
						Host:     "host",
						Change:   3,
						Patchset: 4,
					},
					{
						Host:     "host",
						Change:   1,
						Patchset: 2,
					},
				},
				GitilesCommit: &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "id",
					Ref:     "ref",
				},
				Tags: []*pb.StringPair{
					{
						Key:   "key2",
						Value: "value2",
					},
					{
						Key:   "key1",
						Value: "value1",
					},
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTags(req, b, "")
			assert.Loosely(t, b.Tags, should.Resemble([]*pb.StringPair{
				{
					Key:   "builder",
					Value: "builder",
				},
				{
					Key:   "buildset",
					Value: "commit/gitiles/host/project/+/id",
				},
				{
					Key:   "buildset",
					Value: "patch/gerrit/host/1/2",
				},
				{
					Key:   "buildset",
					Value: "patch/gerrit/host/3/4",
				},
				{
					Key:   "gitiles_ref",
					Value: "ref",
				},
				{
					Key:   "key1",
					Value: "value1",
				},
				{
					Key:   "key2",
					Value: "value2",
				},
			}))
		})
	})

	ftt.Run("setTimeouts", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			b := &pb.Build{}

			setTimeouts(nil, nil, b)
			assert.Loosely(t, b.ExecutionTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 10800,
			}))
			assert.Loosely(t, b.GracePeriod, should.Resemble(&durationpb.Duration{
				Seconds: 30,
			}))
			assert.Loosely(t, b.SchedulingTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 21600,
			}))
		})

		t.Run("request only", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 1,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 2,
				},
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3,
				},
			}
			normalizeSchedule(req)
			b := &pb.Build{}

			setTimeouts(req, nil, b)
			assert.Loosely(t, b.ExecutionTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 1,
			}))
			assert.Loosely(t, b.GracePeriod, should.Resemble(&durationpb.Duration{
				Seconds: 2,
			}))
			assert.Loosely(t, b.SchedulingTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 3,
			}))
		})

		t.Run("config only", func(t *ftt.Test) {
			cfg := &pb.BuilderConfig{
				ExecutionTimeoutSecs: 1,
				ExpirationSecs:       3,
				GracePeriod: &durationpb.Duration{
					Seconds: 2,
				},
			}
			b := &pb.Build{}

			setTimeouts(nil, cfg, b)
			assert.Loosely(t, b.ExecutionTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 1,
			}))
			assert.Loosely(t, b.GracePeriod, should.Resemble(&durationpb.Duration{
				Seconds: 2,
			}))
			assert.Loosely(t, b.SchedulingTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 3,
			}))
		})

		t.Run("override", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 1,
				},
				GracePeriod: &durationpb.Duration{
					Seconds: 2,
				},
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 3,
				},
			}
			normalizeSchedule(req)
			cfg := &pb.BuilderConfig{
				ExecutionTimeoutSecs: 4,
				ExpirationSecs:       6,
				GracePeriod: &durationpb.Duration{
					Seconds: 5,
				},
			}
			b := &pb.Build{}

			setTimeouts(req, cfg, b)
			assert.Loosely(t, b.ExecutionTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 1,
			}))
			assert.Loosely(t, b.GracePeriod, should.Resemble(&durationpb.Duration{
				Seconds: 2,
			}))
			assert.Loosely(t, b.SchedulingTimeout, should.Resemble(&durationpb.Duration{
				Seconds: 3,
			}))
		})
	})

	ftt.Run("scheduleBuilds, one build", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
		})

		assert.Loosely(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Resultdb: &pb.ResultDBSettings{
				Hostname: "rdbHost",
			},
			Swarming: &pb.SwarmingSettings{
				BbagentPackage: &pb.SwarmingSettings_Package{
					PackageName: "bbagent",
					Version:     "bbagent-version",
				},
				KitchenPackage: &pb.SwarmingSettings_Package{
					PackageName: "kitchen",
					Version:     "kitchen-version",
				},
			},
			Backends: []*pb.BackendSetting{
				{
					Target:   "lite://foo-lite",
					Hostname: "foo_hostname",
					Mode:     &pb.BackendSetting_LiteMode_{},
				},
			},
		}), should.BeNil)

		t.Run("builder", func(t *ftt.Test) {
			t.Run("not found", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}
				rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
				assert.Loosely(t, err[0], should.ErrLike("not found"))
				assert.Loosely(t, rsp[0], should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("permission denied", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), should.BeNil)
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}
				rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
				assert.Loosely(t, err[0], should.ErrLike("not found"))
				assert.Loosely(t, rsp[0], should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("directly from dynamic", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
					),
				})

				t.Run("no template in dynamic_builder_template", func(t *ftt.Test) {
					testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{}})
					req := &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					_, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.ErrLike(`builder "project/bucket/builder" is unexpectedly missing its config`))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("has template in dynamic_builder_template", func(t *ftt.Test) {
					testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
						DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{
							Template: &pb.BuilderConfig{
								Backend: &pb.BuilderConfig_Backend{
									Target: "lite://foo-lite",
								},
								Experiments: map[string]int32{
									"luci.buildbucket.backend_alt": 100,
								},
							},
						},
					})
					req := &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					}

					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.BeNil)
					assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreatedBy:  string(userID),
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Id:         9021868963221667745,
						Input:      &pb.Build_Input{},
						Status:     pb.Status_SCHEDULED,
					}))

					buildInDB := &model.Build{ID: 9021868963221667745}
					bInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, buildInDB)}
					assert.Loosely(t, datastore.Get(ctx, buildInDB, bInfra), should.BeNil)
					assert.Loosely(t, bInfra.Proto.Backend, should.Match(&pb.BuildInfra_Backend{
						Caches: []*pb.CacheEntry{
							{
								Name:             "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
								Path:             "cache/builder",
								WaitForWarmCache: &durationpb.Duration{Seconds: 240},
							},
						},
						Config: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"priority": structpb.NewNumberValue(30),
							},
						},
						Hostname: "foo_hostname",
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "lite://foo-lite",
							},
						},
					}))

					tasks := sch.Tasks()
					assert.Loosely(t, tasks, should.HaveLength(2))
					sortTasksByClassName(tasks)
					backendTask, ok := tasks.Payloads()[0].(*taskdefs.CreateBackendBuildTask)
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, backendTask.BuildId, should.Equal(9021868963221667745))
					assert.Loosely(t, tasks.Payloads()[1], should.Resemble(&taskdefs.NotifyPubSubGoProxy{
						BuildId: 9021868963221667745,
						Project: "project",
					}))
				})
			})

			t.Run("static", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
					),
				})

				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Swarming: &pb.Swarming{},
				})
				req := &pb.ScheduleBuildRequest{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				}

				t.Run("not found", func(t *ftt.Test) {
					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.ErrLike("builder not found: \"builder\""))
					assert.Loosely(t, rsp[0], should.BeNil)
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("exists", func(t *ftt.Test) {
					testutil.PutBuilder(ctx, "project", "bucket", "builder", "")
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
						ID: 9021868963221667745,
						Proto: &pb.Build{
							Id: 9021868963221667745,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						},
					}), should.BeNil)

					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.ErrLike("build already exists"))
					assert.Loosely(t, rsp[0], should.BeNil)
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("ok with backend_go exp", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: &pb.BuilderConfig{
							BuildNumbers: pb.Toggle_YES,
							Name:         "builder",
							Experiments:  map[string]int32{bb.ExperimentBackendGo: 100},
							SwarmingHost: "host",
						},
					}), should.BeNil)

					req.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"input key": {
								Kind: &structpb.Value_StringValue{
									StringValue: "input value",
								},
							},
						},
					}
					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.BeNil)
					assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreatedBy:  string(userID),
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Id:         9021868963221667745,
						Input:      &pb.Build_Input{},
						Number:     1,
						Status:     pb.Status_SCHEDULED,
					}))

					// check input.properties and infra are stored in their own Datastore
					// entities and not in Build entity.
					buildInDB := &model.Build{ID: 9021868963221667745}
					assert.Loosely(t, datastore.Get(ctx, buildInDB), should.BeNil)
					assert.Loosely(t, buildInDB.Proto.Input.Properties, should.BeNil)
					assert.Loosely(t, buildInDB.Proto.Infra, should.BeNil)
					inProp := &model.BuildInputProperties{Build: datastore.KeyForObj(ctx, buildInDB)}
					bInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, buildInDB)}
					bs := &model.BuildStatus{Build: datastore.KeyForObj(ctx, buildInDB)}
					assert.Loosely(t, datastore.Get(ctx, inProp, bInfra, bs), should.BeNil)
					assert.Loosely(t, inProp.Proto, should.Resemble(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"input key": {
								Kind: &structpb.Value_StringValue{
									StringValue: "input value",
								},
							},
						},
					}))
					assert.Loosely(t, bInfra.Proto, should.NotBeNil)
					assert.Loosely(t, bs.BuildAddress, should.Equal("project/bucket/builder/1"))
					assert.Loosely(t, bs.Status, should.Equal(pb.Status_SCHEDULED))

					tasks := sch.Tasks()
					assert.Loosely(t, tasks, should.HaveLength(2))
					sortTasksByClassName(tasks)
					assert.Loosely(t, tasks.Payloads()[0], should.Resemble(&taskdefs.CreateSwarmingBuildTask{
						BuildId: 9021868963221667745,
					}))
					assert.Loosely(t, tasks.Payloads()[1], should.Resemble(&taskdefs.NotifyPubSubGoProxy{
						BuildId: 9021868963221667745,
						Project: "project",
					}))
				})

				t.Run("dry_run", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: &pb.BuilderConfig{
							BuildNumbers: pb.Toggle_YES,
							Name:         "builder",
						},
					}), should.BeNil)

					req.DryRun = true
					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.BeNil)
					assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Input: &pb.Build_Input{
							Properties: &structpb.Struct{},
						},
						Tags: []*pb.StringPair{
							{
								Key:   "builder",
								Value: "builder",
							},
						},
						Infra: &pb.BuildInfra{
							Bbagent: &pb.BuildInfra_BBAgent{
								CacheDir:    "cache",
								PayloadPath: "kitchen-checkout",
							},
							Buildbucket: &pb.BuildInfra_Buildbucket{
								Hostname: "app.appspot.com",
								Agent: &pb.BuildInfra_Buildbucket_Agent{
									Input: &pb.BuildInfra_Buildbucket_Agent_Input{},
									Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
										"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
									},
								},
								BuildNumber: true,
							},
							Logdog: &pb.BuildInfra_LogDog{
								Project: "project",
							},
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname: "rdbHost",
							},
							Swarming: &pb.BuildInfra_Swarming{
								Caches: []*pb.BuildInfra_Swarming_CacheEntry{
									{
										Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
										Path: "builder",
										WaitForWarmCache: &durationpb.Duration{
											Seconds: 240,
										},
									},
								},
								Priority: 30,
							},
						},
						Exe:               &pb.Executable{Cmd: []string{"recipes"}},
						SchedulingTimeout: &durationpb.Duration{Seconds: 21600},
						ExecutionTimeout:  &durationpb.Duration{Seconds: 10800},
						GracePeriod:       &durationpb.Duration{Seconds: 30},
					}))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("request ID", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						RequestId: "id",
					}
					testutil.PutBuilder(ctx, "project", "bucket", "builder", "")

					t.Run("deduplication", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(ctx, &model.RequestID{
							ID:      "6d03f5c780125e74ac6cb0f25c5e0b6467ff96c96d98bfb41ba382863ba7707a",
							BuildID: 1,
						}), should.BeNil)

						t.Run("not found", func(t *ftt.Test) {
							rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
							assert.Loosely(t, err[0], should.ErrLike("no such entity"))
							assert.Loosely(t, rsp[0], should.BeNil)
							assert.Loosely(t, sch.Tasks(), should.BeEmpty)
						})

						t.Run("ok", func(t *ftt.Test) {
							assert.Loosely(t, datastore.Put(ctx, &model.Build{
								ID: 1,
								Proto: &pb.Build{
									Builder: &pb.BuilderID{
										Project: "project",
										Bucket:  "bucket",
										Builder: "builder",
									},
									Id: 1,
								},
							}), should.BeNil)

							rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
							assert.Loosely(t, err[0], should.BeNil)
							assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
								Builder: &pb.BuilderID{
									Project: "project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Id:    1,
								Input: &pb.Build_Input{},
							}))
							assert.Loosely(t, sch.Tasks(), should.BeEmpty)
						})
					})

					t.Run("ok", func(t *ftt.Test) {
						rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
						assert.Loosely(t, err[0], should.BeNil)
						assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							CreatedBy:  string(userID),
							CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
							UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
							Id:         9021868963221667745,
							Input:      &pb.Build_Input{},
							Status:     pb.Status_SCHEDULED,
						}))
						assert.Loosely(t, sch.Tasks(), should.HaveLength(2))

						r := &model.RequestID{
							ID: "6d03f5c780125e74ac6cb0f25c5e0b6467ff96c96d98bfb41ba382863ba7707a",
						}
						assert.Loosely(t, datastore.Get(ctx, r), should.BeNil)
						assert.Loosely(t, r, should.Resemble(&model.RequestID{
							ID:         "6d03f5c780125e74ac6cb0f25c5e0b6467ff96c96d98bfb41ba382863ba7707a",
							BuildID:    9021868963221667745,
							CreatedBy:  userID,
							CreateTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
							RequestID:  "id",
						}))
					})

					t.Run("builder description", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(ctx, &model.Builder{
							Parent: model.BucketKey(ctx, "project", "bucket"),
							ID:     "builder",
							Config: &pb.BuilderConfig{
								BuildNumbers:    pb.Toggle_YES,
								Name:            "builder",
								SwarmingHost:    "host",
								DescriptionHtml: "test builder description",
							},
						}), should.BeNil)
						req.Mask = &pb.BuildMask{
							Fields: &fieldmaskpb.FieldMask{
								Paths: []string{
									"builder_info",
								},
							},
						}
						rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
						assert.Loosely(t, err[0], should.BeNil)
						assert.Loosely(t, rsp[0].BuilderInfo.Description, should.Equal("test builder description"))
					})
				})
			})
		})

		t.Run("template build ID", func(t *ftt.Test) {
			t.Run("not found", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1000,
				}
				rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
				assert.Loosely(t, err[0], should.ErrLike("not found"))
				assert.Loosely(t, rsp[0], should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("permission denied", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID: 1000,
					Proto: &pb.Build{
						Id: 1000,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), should.BeNil)
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
				}
				rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
				assert.Loosely(t, err[0], should.ErrLike("not found"))
				assert.Loosely(t, rsp[0], should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("not retriable", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
					),
				})
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Name:     "bucket",
					Swarming: &pb.Swarming{},
				})
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID: 1000,
					Proto: &pb.Build{
						Id: 1000,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Retriable: pb.Trinary_NO,
					},
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(ctx, &model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder",
					Config: &pb.BuilderConfig{
						BuildNumbers: pb.Toggle_YES,
						Name:         "builder",
						SwarmingHost: "host",
					},
				}), should.BeNil)
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1000,
				}

				rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
				assert.Loosely(t, err[0], should.ErrLike("build 1000 is not retriable"))
				assert.Loosely(t, rsp[0], should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("ok", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
					),
				})
				testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
					Name:     "bucket",
					Swarming: &pb.Swarming{},
				})
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID: 1000,
					Proto: &pb.Build{
						Id: 1000,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
				}), should.BeNil)
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1000,
				}

				t.Run("not found", func(t *ftt.Test) {
					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.ErrLike("builder not found: \"builder\""))
					assert.Loosely(t, rsp[0], should.BeNil)
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("ok", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(ctx, &model.Builder{
						Parent: model.BucketKey(ctx, "project", "bucket"),
						ID:     "builder",
						Config: &pb.BuilderConfig{
							BuildNumbers: pb.Toggle_YES,
							Name:         "builder",
							SwarmingHost: "host",
						},
					}), should.BeNil)
					rsp, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{req}, nil)
					assert.Loosely(t, err[0], should.BeNil)
					assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						CreatedBy:  string(userID),
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Id:         9021868963221667745,
						Input:      &pb.Build_Input{},
						Number:     1,
						Status:     pb.Status_SCHEDULED,
					}))
					assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
				})
			})
		})
	})

	ftt.Run("scheduleBuilds, many builds", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(0)))
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
				authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
			),
		})
		globalCfg := &pb.SettingsCfg{
			Resultdb: &pb.ResultDBSettings{
				Hostname: "rdbHost",
			},
			Swarming: &pb.SwarmingSettings{
				BbagentPackage: &pb.SwarmingSettings_Package{
					PackageName: "bbagent",
					Version:     "bbagent-version",
				},
				KitchenPackage: &pb.SwarmingSettings_Package{
					PackageName: "kitchen",
					Version:     "kitchen-version",
				},
			},
		}
		assert.NoErr(t, config.SetTestSettingsCfg(ctx, globalCfg))

		testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{
			Name:     "bucket",
			Swarming: &pb.Swarming{},
		})
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
			ID: 1000,
			Proto: &pb.Build{
				Id: 1000,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Build{
			ID: 9999,
			Proto: &pb.Build{
				Id: 1001,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "maxConc",
				},
			},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Builder{
			Parent: model.BucketKey(ctx, "project", "bucket"),
			ID:     "builder",
			Config: &pb.BuilderConfig{
				BuildNumbers: pb.Toggle_YES,
				Name:         "builder",
				SwarmingHost: "host",
			},
		}), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &model.Builder{
			Parent: model.BucketKey(ctx, "project", "bucket"),
			ID:     "maxConc",
			Config: &pb.BuilderConfig{
				BuildNumbers:        pb.Toggle_YES,
				Name:                "maxConc",
				SwarmingHost:        "host",
				MaxConcurrentBuilds: 2,
			},
		}), should.BeNil)

		t.Run("mixed", func(t *ftt.Test) {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					DryRun: true,
				},
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					DryRun: false,
				},
			}

			rsp, merr := scheduleBuilds(ctx, reqs, nil)
			assert.Loosely(t, merr, should.HaveLength(3))
			assert.Loosely(t, rsp, should.HaveLength(3))
			for i := range 3 {
				assert.Loosely(t, rsp[i], should.BeNil)
				assert.Loosely(t, merr[i], should.ErrLike("all requests must have the same dry_run value"))
			}
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("one", func(t *ftt.Test) {
			reqs := []*pb.ScheduleBuildRequest{
				{
					TemplateBuildId: 1000,
					Tags: []*pb.StringPair{
						{
							Key:   "buildset",
							Value: "buildset",
						},
					},
				},
			}

			rsp, merr := scheduleBuilds(ctx, reqs, nil)
			assert.Loosely(t, merr.First(), should.BeNil)
			assert.Loosely(t, merr, should.HaveLength(1))
			assert.Loosely(t, rsp, should.HaveLength(1))
			assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				CreatedBy:  string(userID),
				CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Id:         9021868963221667745,
				Input:      &pb.Build_Input{},
				Number:     1,
				Status:     pb.Status_SCHEDULED,
			}))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(2))

			ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ind, should.Resemble([]*model.TagIndexEntry{
				{
					BuildID:     9021868963221667745,
					BucketID:    "project/bucket",
					CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
				},
			}))
		})

		t.Run("one with max_concurrent_builds", func(t *ftt.Test) {
			reqs := []*pb.ScheduleBuildRequest{
				{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "maxConc",
					},
				},
			}

			rsp, merr := scheduleBuilds(ctx, reqs, nil)
			assert.Loosely(t, merr.First(), should.BeNil)
			assert.Loosely(t, merr, should.HaveLength(1))
			assert.Loosely(t, rsp, should.HaveLength(1))
			assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "maxConc",
				},
				CreatedBy:  string(userID),
				CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Id:         9021868963221667745,
				Number:     1,
				Input:      &pb.Build_Input{},
				Status:     pb.Status_SCHEDULED,
			}))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
			assert.Loosely(t, sch.Tasks()[0].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), should.Equal(9021868963221667745))
			assert.Loosely(t, sch.Tasks()[1].Payload.(*taskdefs.PushPendingBuildTask).GetBuildId(), should.Equal(9021868963221667745))
		})

		t.Run("one with custom metrics", func(t *ftt.Test) {
			globalCfg := &pb.SettingsCfg{
				Resultdb: &pb.ResultDBSettings{
					Hostname: "rdbHost",
				},
				Swarming: &pb.SwarmingSettings{
					BbagentPackage: &pb.SwarmingSettings_Package{
						PackageName: "bbagent",
						Version:     "bbagent-version",
					},
					KitchenPackage: &pb.SwarmingSettings_Package{
						PackageName: "kitchen",
						Version:     "kitchen-version",
					},
				},
				CustomMetrics: []*pb.CustomMetric{
					{
						Name:        "chrome/infra/custom/builds/created",
						ExtraFields: []string{"os"},
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED,
						},
					},
					{
						Name:        "chrome/infra/custom/builds/completed",
						ExtraFields: []string{"os"},
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED,
						},
					},
					{
						Name: "chrome/infra/custom/builds/max_age",
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED,
						},
					},
				},
			}
			assert.NoErr(t, config.SetTestSettingsCfg(ctx, globalCfg))
			ctx, _ = metrics.WithCustomMetrics(ctx, globalCfg)
			cm1 := &pb.CustomMetricDefinition{
				Name:       "chrome/infra/custom/builds/created",
				Predicates: []string{`build.tags.get_value("os")!=""`},
				ExtraFields: map[string]string{
					"os": `build.tags.get_value("os")`,
				},
			}
			cm2 := &pb.CustomMetricDefinition{
				Name:        "chrome/infra/custom/builds/completed",
				Predicates:  []string{`build.tags.get_value("os")!=""`},
				ExtraFields: map[string]string{"os": `build.tags.get_value("os")`},
			}
			cm3 := &pb.CustomMetricDefinition{
				Name:        "chrome/infra/custom/builds/missing",
				Predicates:  []string{`build.tags.get_value("os")!=""`},
				ExtraFields: map[string]string{"os": `build.tags.get_value("os")`},
			}
			cm4 := &pb.CustomMetricDefinition{
				Name:       "chrome/infra/custom/builds/max_age",
				Predicates: []string{`build.tags.get_value("buildset")!=""`},
			}
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
				Config: &pb.BuilderConfig{
					BuildNumbers:            pb.Toggle_YES,
					Name:                    "builder",
					SwarmingHost:            "host",
					CustomMetricDefinitions: []*pb.CustomMetricDefinition{cm1, cm2, cm3, cm4},
				},
			}), should.BeNil)
			reqs := []*pb.ScheduleBuildRequest{
				{
					TemplateBuildId: 1000,
					Tags: []*pb.StringPair{
						{
							Key:   "buildset",
							Value: "buildset",
						},
						{
							Key:   "os",
							Value: "Linux",
						},
					},
				},
			}
			rsp, merr := scheduleBuilds(ctx, reqs, nil)
			assert.Loosely(t, merr.First(), should.BeNil)
			assert.Loosely(t, merr, should.HaveLength(1))
			assert.Loosely(t, rsp, should.HaveLength(1))
			assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				CreatedBy:  string(userID),
				CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
				Id:         9021868963221667745,
				Input:      &pb.Build_Input{},
				Number:     1,
				Status:     pb.Status_SCHEDULED,
			}))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(2))

			ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ind, should.Resemble([]*model.TagIndexEntry{
				{
					BuildID:     9021868963221667745,
					BucketID:    "project/bucket",
					CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
				},
			}))
			bld := &model.Build{ID: 9021868963221667745}
			assert.Loosely(t, datastore.Get(ctx, bld), should.BeNil)
			assert.Loosely(t, len(bld.CustomMetrics), should.Equal(3))
			assert.Loosely(t, bld.CustomMetrics[0].Base, should.Equal(pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED))
			assert.Loosely(t, bld.CustomMetrics[0].Metric, should.Resemble(cm1))
			assert.Loosely(t, bld.CustomMetrics[1].Base, should.Equal(pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED))
			assert.Loosely(t, bld.CustomMetrics[1].Metric, should.Resemble(cm2))
			assert.Loosely(t, bld.CustomMetrics[2].Base, should.Equal(pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED))
			assert.Loosely(t, bld.CustomMetrics[2].Metric, should.Resemble(cm4))
			assert.Loosely(t, bld.CustomBuilderMaxAgeMetrics, should.Resemble([]string{"chrome/infra/custom/builds/max_age"}))
		})

		t.Run("many", func(t *ftt.Test) {
			t.Run("one of TemplateBuildId builds not found", func(t *ftt.Test) {
				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1001,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
				}

				rsp, err := scheduleBuilds(ctx, reqs, nil)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err[0], should.BeNil)
				assert.Loosely(t, err[1], should.ErrLike(`requested resource not found or "user:caller@example.com" does not have permission to view it`))
				assert.Loosely(t, err[2], should.BeNil)
				assert.Loosely(t, rsp, should.Resemble([]*pb.Build{
					{
						Id:         9021868963222163313,
						Builder:    &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"},
						Number:     1,
						CreatedBy:  string(userID),
						Status:     pb.Status_SCHEDULED,
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Input:      &pb.Build_Input{},
					},
					nil,
					{
						Id:         9021868963222163297,
						Builder:    &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"},
						Number:     2,
						CreatedBy:  string(userID),
						Status:     pb.Status_SCHEDULED,
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Input:      &pb.Build_Input{},
					},
				}))
			})

			t.Run("one of builds missing builderCfg", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID: 1010,
					Proto: &pb.Build{
						Id: 1010,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "miss_builder_cfg",
						},
					},
				}), should.BeNil)

				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1010,
					},
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
				}

				rsp, err := scheduleBuilds(ctx, reqs, nil)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err[0], should.BeNil)
				assert.Loosely(t, err[1], should.ErrLike(`builder not found: "miss_builder_cfg"`))
				assert.Loosely(t, err[2], should.BeNil)
				assert.Loosely(t, rsp, should.Resemble([]*pb.Build{
					{
						Id:         9021868963222163313,
						Builder:    &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"},
						Number:     1,
						CreatedBy:  string(userID),
						Status:     pb.Status_SCHEDULED,
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Input:      &pb.Build_Input{},
					},
					nil,
					{
						Id:         9021868963222163297,
						Builder:    &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"},
						Number:     2,
						CreatedBy:  string(userID),
						Status:     pb.Status_SCHEDULED,
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Input:      &pb.Build_Input{},
					},
				}))
			})

			t.Run("one of builds failed in `createBuilds` part", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Build{
					ID: 1011,
					Proto: &pb.Build{
						Id: 1011,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder_with_rdb",
						},
					},
				}), should.BeNil)
				bqExports := []*rdbPb.BigQueryExport{}
				assert.Loosely(t, datastore.Put(ctx, &model.Builder{
					Parent: model.BucketKey(ctx, "project", "bucket"),
					ID:     "builder_with_rdb",
					Config: &pb.BuilderConfig{
						BuildNumbers: pb.Toggle_YES,
						Name:         "builder_with_rdb",
						SwarmingHost: "host",
						Resultdb: &pb.BuilderConfig_ResultDB{
							Enable:    true,
							BqExports: bqExports,
						},
					},
				}), should.BeNil)

				ctl := gomock.NewController(t)
				defer ctl.Finish()
				mockRdbClient := rdbPb.NewMockRecorderClient(ctl)
				ctx = resultdb.SetMockRecorder(ctx, mockRdbClient)
				deadline := testclock.TestRecentTimeUTC.Add(time.Second * 10800).Add(time.Second * 21600)
				ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
				mockRdbClient.EXPECT().CreateInvocation(gomock.Any(), luciCmProto.MatcherEqual(
					&rdbPb.CreateInvocationRequest{
						InvocationId: "build-9021868963221610321",
						Invocation: &rdbPb.Invocation{
							BigqueryExports:  bqExports,
							ProducerResource: "//app.appspot.com/builds/9021868963221610321",
							Realm:            "project:bucket",
							Deadline:         timestamppb.New(deadline),
							IsExportRoot:     true,
						},
						RequestId: "build-9021868963221610321",
					}), gomock.Any()).Return(nil, grpcStatus.Error(codes.Internal, "internal error"))

				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1011,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
				}

				rsp, err := scheduleBuilds(ctx, reqs, nil)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err[0], should.BeNil)
				assert.Loosely(t, err[1], should.ErrLike("failed to create the invocation for build id: 9021868963221610321: rpc error: code = Internal desc = internal error"))
				assert.Loosely(t, err[2], should.BeNil)
				assert.Loosely(t, rsp, should.Resemble([]*pb.Build{
					{
						Id:         9021868963221610337,
						Builder:    &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"},
						Number:     1,
						CreatedBy:  string(userID),
						Status:     pb.Status_SCHEDULED,
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Input:      &pb.Build_Input{},
					},
					nil,
					{
						Id:         9021868963221610305,
						Builder:    &pb.BuilderID{Project: "project", Bucket: "bucket", Builder: "builder"},
						Number:     2,
						CreatedBy:  string(userID),
						Status:     pb.Status_SCHEDULED,
						CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
						Input:      &pb.Build_Input{},
					},
				}))
			})

			t.Run("scheduleBuilds with parent_build_id", func(t *ftt.Test) {
				testutil.PutBucket(ctx, "project", "bucket1", &pb.Bucket{
					Name:     "bucket1",
					Swarming: &pb.Swarming{},
				})
				testutil.PutBuilder(ctx, "project", "bucket1", "builder", "")
				testutil.PutBucket(ctx, "project", "parent_bucket", &pb.Bucket{
					Name:     "parent_bucket",
					Swarming: &pb.Swarming{},
				})
				p1 := &model.Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "parent_bucket",
							Builder: "builder",
						},
						Status: pb.Status_STARTED,
					},
				}
				pi1 := &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, p1),
				}
				p2 := &model.Build{
					ID: 2,
					Proto: &pb.Build{
						Id: 2,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "parent_bucket",
							Builder: "builder",
						},
						Status: pb.Status_STARTED,
					},
				}
				pi2 := &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, p2),
				}
				assert.Loosely(t, datastore.Put(ctx, p1, p2, pi1, pi2), should.BeNil)

				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
						ParentBuildId: 1,
					},
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket1",
							Builder: "builder",
						},
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
						ParentBuildId: 2,
					},
				}
				t.Run("no permission to update parent to include children", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAddAsChild),
							// Because it's using TemplateBuildId
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
							authtest.MockPermission(userID, "project:bucket1", bbperms.BuildsAdd),
							authtest.MockPermission(userID, "project:bucket1", bbperms.BuildsAddAsChild),
							authtest.MockPermission(userID, "project:parent_bucket", bbperms.BuildersGet),
						),
					})
					_, merr := scheduleBuilds(ctx, reqs, nil)
					assert.Loosely(t, merr, should.NotBeNil)
					assert.Loosely(t, merr[0], grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, merr[1], grpccode.ShouldBe(codes.PermissionDenied))
				})

				t.Run("one of builds doesn't have permission to set parent_build_id", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAddAsChild),
							// Because it's using TemplateBuildId
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
							authtest.MockPermission(userID, "project:bucket1", bbperms.BuildersGet),
							authtest.MockPermission(userID, "project:parent_bucket", bbperms.BuildsIncludeChild),
						),
					})
					_, merr := scheduleBuilds(ctx, reqs, nil)
					assert.Loosely(t, merr, should.NotBeNil)
					assert.Loosely(t, merr[0], should.BeNil)
					assert.Loosely(t, merr[1], grpccode.ShouldBe(codes.PermissionDenied))
				})
				t.Run("OK", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity: userID,
						FakeDB: authtest.NewFakeDB(
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAddAsChild),
							authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
							authtest.MockPermission(userID, "project:bucket1", bbperms.BuildsAdd),
							authtest.MockPermission(userID, "project:bucket1", bbperms.BuildsAddAsChild),
							authtest.MockPermission(userID, "project:parent_bucket", bbperms.BuildsIncludeChild),
						),
					})
					_, merr := scheduleBuilds(ctx, reqs, nil)
					assert.Loosely(t, merr.First(), should.BeNil)
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				reqs := []*pb.ScheduleBuildRequest{
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 1000,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
					{
						TemplateBuildId: 9999,
						Tags: []*pb.StringPair{
							{
								Key:   "buildset",
								Value: "buildset",
							},
						},
					},
				}

				rsp, merr := scheduleBuilds(ctx, reqs, nil)
				assert.Loosely(t, merr.First(), should.BeNil)
				assert.Loosely(t, merr, should.HaveLength(3))
				assert.Loosely(t, rsp, should.HaveLength(3))
				assert.Loosely(t, rsp[0], should.Resemble(&pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy:  string(userID),
					UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Id:         9021868963221610337,
					Input:      &pb.Build_Input{},
					Number:     1,
					Status:     pb.Status_SCHEDULED,
				}))
				assert.Loosely(t, rsp[1], should.Resemble(&pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					CreatedBy:  string(userID),
					UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Id:         9021868963221610321,
					Input:      &pb.Build_Input{},
					Number:     2,
					Status:     pb.Status_SCHEDULED,
				}))
				assert.Loosely(t, rsp[2], should.Resemble(&pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "maxConc",
					},
					CreatedBy:  string(userID),
					UpdateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					CreateTime: timestamppb.New(testclock.TestRecentTimeUTC),
					Id:         9021868963221610305,
					Input:      &pb.Build_Input{},
					Number:     1,
					Status:     pb.Status_SCHEDULED,
				}))
				assert.Loosely(t, sch.Tasks(), should.HaveLength(6))
				sum := 0
				for _, task := range sch.Tasks() {
					switch task.Payload.(type) {
					case *taskdefs.NotifyPubSubGoProxy:
						sum += 2
					case *taskdefs.PushPendingBuildTask:
						sum += 4
					case *taskdefs.CreateSwarmingBuildTask:
						sum += 8
					default:
						panic("invalid task payload")
					}
				}
				assert.Loosely(t, sum, should.Equal(26))

				ind, err := model.SearchTagIndex(ctx, "buildset", "buildset")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ind, should.Resemble([]*model.TagIndexEntry{
					{
						BuildID:     9021868963221610337,
						BucketID:    "project/bucket",
						CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
					},
					{
						BuildID:     9021868963221610321,
						BucketID:    "project/bucket",
						CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
					},
					{
						BuildID:     9021868963221610305,
						BucketID:    "project/bucket",
						CreatedTime: datastore.RoundTime(testclock.TestRecentTimeUTC),
					},
				}))
			})
		})

		t.Run("schedule in shadow", func(t *ftt.Test) {
			testutil.PutBucket(ctx, "project", "bucket", &pb.Bucket{Swarming: &pb.Swarming{}, Shadow: "bucket.shadow"})
			testutil.PutBucket(ctx, "project", "bucket.shadow", &pb.Bucket{DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{}})
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				Parent: model.BucketKey(ctx, "project", "bucket"),
				ID:     "builder",
				Config: &pb.BuilderConfig{
					Name:           "builder",
					ServiceAccount: "sa@chops-service-accounts.iam.gserviceaccount.com",
					SwarmingHost:   "swarming.appspot.com",
					Dimensions:     []string{"pool:pool1"},
					Properties:     `{"a":"b","b":"b"}`,
					ShadowBuilderAdjustments: &pb.BuilderConfig_ShadowBuilderAdjustments{
						ServiceAccount: "shadow@chops-service-accounts.iam.gserviceaccount.com",
						Pool:           "pool2",
						Properties:     `{"a":"b2","c":"c"}`,
						Dimensions: []string{
							"pool:pool2",
						},
					},
				},
			}), should.BeNil)

			t.Run("no permission", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
						authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildersGet),
					),
				})
				reqs := []*pb.ScheduleBuildRequest{
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{},
					},
				}
				_, err := scheduleBuilds(ctx, reqs, nil)
				assert.Loosely(t, err, should.ErrLike(`does not have permission "buildbucket.builds.add"`))
			})

			t.Run("inherit from parent", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsIncludeChild),
						authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildsAdd),
						authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildsAddAsChild),
					),
				})

				parentBuild := &model.Build{
					ID: 1234,
					Proto: &pb.Build{
						Id: 1234,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Exe: &pb.Executable{
							CipdPackage: "some/package",
						},
						Status: pb.Status_STARTED,
					},
				}
				parentBuildInfra := &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, parentBuild),
					Proto: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Agent: &pb.BuildInfra_Buildbucket_Agent{
								Input: &pb.BuildInfra_Buildbucket_Agent_Input{
									Data: map[string]*pb.InputDataRef{
										"some-key": {},
									},
								},
								Source: &pb.BuildInfra_Buildbucket_Agent_Source{
									DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
										Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
											Package: "infra/tools/luci/bbagent/${platform}",
											Version: "canary-version",
											Server:  "cipd server",
										},
									},
								},
							},
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, parentBuild, parentBuildInfra), should.BeNil)

				blds, err := scheduleBuilds(ctx, []*pb.ScheduleBuildRequest{
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{
							InheritFromParent: true,
						},
						ParentBuildId: 1234,
						Mask:          &pb.BuildMask{AllFields: true},
					},
				}, nil)
				assert.Loosely(t, err[0], should.BeNil)

				expectedAgent := proto.Clone(parentBuildInfra.Proto.Buildbucket.Agent).(*pb.BuildInfra_Buildbucket_Agent)
				expectedAgent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
					"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
				}
				expectedAgent.CipdPackagesCache = &pb.CacheEntry{
					Name: fmt.Sprintf("cipd_cache_%x", sha256.Sum256([]byte("shadow@chops-service-accounts.iam.gserviceaccount.com"))),
					Path: "cipd_cache",
				}
				assert.That(t, blds[0].Infra.Buildbucket.Agent, should.Match(expectedAgent))
				assert.That(t, blds[0].Exe, should.Match(parentBuild.Proto.Exe))
			})

			t.Run("one shadow, one original, and one with no shadow bucket", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildsAdd),
						authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildsGet),
						authtest.MockPermission(userID, "project:bucket.shadow", bbperms.BuildsAdd),
					),
				})
				reqs := []*pb.ScheduleBuildRequest{
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{},
					},
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
					},
					{
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket.shadow",
							Builder: "builder",
						},
						ShadowInput: &pb.ScheduleBuildRequest_ShadowInput{},
					},
				}
				blds, err := scheduleBuilds(ctx, reqs, nil)
				assert.Loosely(t, err[0], should.BeNil)
				assert.Loosely(t, err[1], should.BeNil)
				assert.Loosely(t, err[2], should.ErrLike("scheduling a shadow build in the original bucket is not allowed"))
				assert.Loosely(t, len(blds), should.Equal(3))
				assert.Loosely(t, blds[2], should.BeNil)
			})
		})
	})

	ftt.Run("structContains", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			assert.Loosely(t, structContains(nil, nil), should.BeTrue)
		})

		t.Run("nil struct", func(t *ftt.Test) {
			path := []string{"path"}
			assert.Loosely(t, structContains(nil, path), should.BeFalse)
		})

		t.Run("nil path", func(t *ftt.Test) {
			s := &structpb.Struct{}
			assert.Loosely(t, structContains(s, nil), should.BeTrue)
		})

		t.Run("one component", func(t *ftt.Test) {
			s := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": {
						Kind: &structpb.Value_StringValue{
							StringValue: "value",
						},
					},
				},
			}
			path := []string{"key"}
			assert.Loosely(t, structContains(s, path), should.BeTrue)
		})

		t.Run("many components", func(t *ftt.Test) {
			s := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key1": {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"key2": {
										Kind: &structpb.Value_StructValue{
											StructValue: &structpb.Struct{
												Fields: map[string]*structpb.Value{
													"key3": {
														Kind: &structpb.Value_StringValue{
															StringValue: "value",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			path := []string{"key1", "key2", "key3"}
			assert.Loosely(t, structContains(s, path), should.BeTrue)
		})

		t.Run("excess component", func(t *ftt.Test) {
			s := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key1": {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"key2": {
										Kind: &structpb.Value_StringValue{
											StringValue: "value",
										},
									},
								},
							},
						},
					},
				},
			}
			path := []string{"key1"}
			assert.Loosely(t, structContains(s, path), should.BeTrue)
		})
	})

	ftt.Run("validateSchedule", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = installTestSecret(ctx)

		t.Run("nil", func(t *ftt.Test) {
			err := validateSchedule(ctx, nil, nil, nil)
			assert.Loosely(t, err, should.ErrLike("builder or template_build_id is required"))
		})

		t.Run("empty", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{}
			err := validateSchedule(ctx, req, nil, nil)
			assert.Loosely(t, err, should.ErrLike("builder or template_build_id is required"))
		})

		t.Run("request ID", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				RequestId:       "request/id",
				TemplateBuildId: 1,
			}
			err := validateSchedule(ctx, req, nil, nil)
			assert.Loosely(t, err, should.ErrLike("request_id cannot contain"))
		})

		t.Run("builder ID", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Builder: &pb.BuilderID{},
			}
			err := validateSchedule(ctx, req, nil, nil)
			assert.Loosely(t, err, should.ErrLike("project must match"))
		})

		t.Run("dimensions", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("dimensions"))
			})

			t.Run("expiration", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{},
								Key:        "key",
								Value:      "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.BeNil)
				})

				t.Run("nanos", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Nanos: 1,
								},
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("nanos must not be specified"))
				})

				t.Run("seconds", func(t *ftt.Test) {
					t.Run("negative", func(t *ftt.Test) {
						req := &pb.ScheduleBuildRequest{
							Dimensions: []*pb.RequestedDimension{
								{
									Expiration: &durationpb.Duration{
										Seconds: -60,
									},
									Key:   "key",
									Value: "value",
								},
							},
							TemplateBuildId: 1,
						}
						err := validateSchedule(ctx, req, nil, nil)
						assert.Loosely(t, err, should.ErrLike("seconds must not be negative"))
					})

					t.Run("whole minute", func(t *ftt.Test) {
						req := &pb.ScheduleBuildRequest{
							Dimensions: []*pb.RequestedDimension{
								{
									Expiration: &durationpb.Duration{
										Seconds: 1,
									},
									Key:   "key",
									Value: "value",
								},
							},
							TemplateBuildId: 1,
						}
						err := validateSchedule(ctx, req, nil, nil)
						assert.Loosely(t, err, should.ErrLike("seconds must be a multiple of 60"))
					})
				})

				t.Run("ok", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Expiration: &durationpb.Duration{
									Seconds: 60,
								},
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("key", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("the key cannot be empty"))
				})

				t.Run("caches", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "caches",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("caches may only be specified in builder configs"))
				})

				t.Run("pool", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "pool",
								Value: "value",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("pool may only be specified in builder configs"))
				})

				t.Run("ok", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key: "key1",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("parent", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity: userID,
					FakeDB: authtest.NewFakeDB(
						authtest.MockPermission(userID, "project:bucket", bbperms.BuildersGet),
					),
				})
				tk, err := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_BUILD)
				assert.Loosely(t, err, should.BeNil)
				testutil.PutBucket(ctx, "project", "bucket", nil)
				pBld := &model.Build{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_STARTED,
					},
					UpdateToken: tk,
				}
				pBldInfra := &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, pBld),
				}
				assert.Loosely(t, datastore.Put(ctx, pBld, pBldInfra), should.BeNil)

				t.Run("missing parent", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId:  1,
						CanOutliveParent: pb.Trinary_NO,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("can_outlive_parent is specified without parent"))
				})

				t.Run("schedule no parent build", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Dimensions: []*pb.RequestedDimension{
							{
								Key:   "key",
								Value: "value",
							},
						},
						TemplateBuildId:  1,
						CanOutliveParent: pb.Trinary_UNSET,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.BeNil)
				})

				t.Run("use parent build token", func(t *ftt.Test) {
					t.Run("ended parent", func(t *ftt.Test) {
						pBld.Proto.Status = pb.Status_SUCCESS
						assert.Loosely(t, datastore.Put(ctx, pBld), should.BeNil)
						assert.Loosely(t, datastore.Put(ctx, pBld), should.BeNil)
						ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(bb.BuildbucketTokenHeader, tk))
						ps, err := validateParents(ctx, nil, nil)
						assert.Loosely(t, err, should.ErrLike("1 has ended, cannot add child to it"))
						assert.Loosely(t, ps, should.BeNil)
					})

					t.Run("OK", func(t *ftt.Test) {
						pBld.Proto.Status = pb.Status_STARTED
						assert.Loosely(t, datastore.Put(ctx, pBld), should.BeNil)
						ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(bb.BuildbucketTokenHeader, tk))
						pMap, err := validateParents(ctx, nil, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, pMap.fromToken.bld.Proto.Id, should.Equal(1))
					})

					t.Run("both token and id are provided", func(t *ftt.Test) {
						ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(bb.BuildbucketTokenHeader, tk))
						_, err := validateParents(ctx, []int64{1}, nil)
						assert.Loosely(t, err, should.ErrLike("parent buildbucket token and parent_build_id are mutually exclusive"))
					})
				})

				t.Run("use parent_build_id", func(t *ftt.Test) {
					t.Run("no permission to set parent_build_id", func(t *ftt.Test) {
						req := &pb.ScheduleBuildRequest{
							Dimensions: []*pb.RequestedDimension{
								{
									Key:   "key",
									Value: "value",
								},
							},
							TemplateBuildId:  1,
							CanOutliveParent: pb.Trinary_NO,
							ParentBuildId:    1,
						}
						op := &scheduleBuildOp{
							Reqs: []*pb.ScheduleBuildRequest{req},
							Parents: &parentsMap{
								fromRequests: map[int64]*parent{
									1: {bld: pBld},
								},
							},
						}
						_, _, err := validateScheduleBuild(ctx, op, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					})

					t.Run("missing build entities", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(ctx, &model.Build{
							Proto: &pb.Build{
								Id: 333,
								Builder: &pb.BuilderID{
									Project: "project",
									Bucket:  "bucket",
									Builder: "builder",
								},
								Status: pb.Status_STARTED,
							},
						}), should.BeNil)
						ctx = auth.WithState(ctx, &authtest.FakeState{
							Identity: userID,
							FakeDB: authtest.NewFakeDB(
								authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
								authtest.MockPermission(userID, "project:bucket", bbperms.BuildsIncludeChild),
							),
						})
						pMap, err := validateParents(ctx, []int64{333, 444}, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, pMap.fromRequests[333].err, grpccode.ShouldBe(codes.NotFound))
						assert.Loosely(t, pMap.fromRequests[444].err, grpccode.ShouldBe(codes.NotFound))
					})

					t.Run("OK", func(t *ftt.Test) {
						ctx = auth.WithState(ctx, &authtest.FakeState{
							Identity: userID,
							FakeDB: authtest.NewFakeDB(
								authtest.MockPermission(userID, "project:bucket", bbperms.BuildsGet),
								authtest.MockPermission(userID, "project:bucket", bbperms.BuildsIncludeChild),
							),
						})
						pMap, err := validateParents(ctx, []int64{1}, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, pMap.fromRequests[1].bld.Proto.Id, should.Equal(1))
					})
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{
							Key:   "key",
							Value: "value",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("empty value & non-value", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Dimensions: []*pb.RequestedDimension{
						{
							Key:   "req_key",
							Value: "value",
						},
						{
							Key: "req_key",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike(`dimensions: contain both empty and non-empty value for the same key - "req_key"`))
			})
		})

		t.Run("exe", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Exe:             &pb.Executable{},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("package", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Exe: &pb.Executable{
						CipdPackage: "package",
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("cipd_package must not be specified"))
			})

			t.Run("version", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Exe: &pb.Executable{
							CipdVersion: "invalid!",
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("cipd_version"))
				})

				t.Run("valid", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						Exe: &pb.Executable{
							CipdVersion: "valid",
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})

		t.Run("gerrit changes", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges:   []*pb.GerritChange{},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("unspecified", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("gerrit_changes"))
			})

			t.Run("change", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "host",
							Patchset: 1,
							Project:  "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("change must be specified"))
			})

			t.Run("host", func(t *ftt.Test) {
				t.Run("not specified", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						GerritChanges: []*pb.GerritChange{
							{
								Change:   1,
								Patchset: 1,
								Project:  "project",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("host must be specified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						GerritChanges: []*pb.GerritChange{
							{
								Change:   1,
								Host:     "https://somehost", // host should not include the protocol.
								Patchset: 1,
								Project:  "project",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("host does not match pattern"))
				})
				t.Run("too long", func(t *ftt.Test) {
					req := &pb.ScheduleBuildRequest{
						GerritChanges: []*pb.GerritChange{
							{
								Change:   1,
								Host:     strings.Repeat("h", 256),
								Patchset: 1,
								Project:  "project",
							},
						},
						TemplateBuildId: 1,
					}
					err := validateSchedule(ctx, req, nil, nil)
					assert.Loosely(t, err, should.ErrLike("host must not exceed 255 characters"))
				})
			})

			t.Run("patchset", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:  1,
							Host:    "host",
							Project: "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("patchset must be specified"))
			})

			t.Run("project", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:   1,
							Host:     "host",
							Patchset: 1,
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("project must be specified"))
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					GerritChanges: []*pb.GerritChange{
						{
							Change:   1,
							Host:     "host",
							Patchset: 1,
							Project:  "project",
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("gitiles commit", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				GitilesCommit: &pb.GitilesCommit{
					Host: "example.com",
				},
				TemplateBuildId: 1,
			}
			err := validateSchedule(ctx, req, nil, nil)
			assert.Loosely(t, err, should.ErrLike("gitiles_commit"))
		})

		t.Run("notify", func(t *ftt.Test) {
			ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "project")
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				psclient.Close()
				psserver.Close()
			}()
			tpc, err := psclient.CreateTopic(ctx, "topic")
			tpc.IAM()
			assert.Loosely(t, err, should.BeNil)
			ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
				"has_perm_on_pubsub_callback_topic": cachingtest.NewBlobCache(),
			})
			t.Run("empty", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Notify:          &pb.NotificationConfig{},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("notify"))
			})

			t.Run("pubsub topic", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						UserData: []byte("user data"),
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("pubsub_topic"))
			})

			t.Run("user data", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						PubsubTopic: "projects/project/topics/topic",
						UserData:    make([]byte, 4097),
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("user_data"))
			})

			t.Run("ok - pubsub topic perm cached", func(t *ftt.Test) {
				cache := caching.GlobalCache(ctx, "has_perm_on_pubsub_callback_topic")
				err := cache.Set(ctx, "projects/project/topics/topic", []byte{1}, 10*time.Hour)
				assert.Loosely(t, err, should.BeNil)
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						PubsubTopic: "projects/project/topics/topic",
						UserData:    []byte("user data"),
					},
					TemplateBuildId: 1,
				}
				err = validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("ok - pubsub topic perm not cached", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Notify: &pb.NotificationConfig{
						PubsubTopic: "projects/project/topics/topic",
						UserData:    []byte("user data"),
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				// "cloud.google.com/go/pubsub/pstest" lib doesn't expose a way to mock
				// IAM policy check. Therefore, only check if our `validateSchedule`
				// tries to call `topic.IAM().TestPermissions()` and get the expected
				// `Unimplemented` err msg.
				assert.Loosely(t, err, should.ErrLike("Unimplemented desc = unknown service google.iam.v1.IAMPolicy"))
				// The bad result should not be cached.
				cache := caching.GlobalCache(ctx, "has_perm_on_pubsub_callback_topic")
				_, err = cache.Get(ctx, "projects/project/topics/topic")
				assert.Loosely(t, err, should.ErrLike(caching.ErrCacheMiss))
			})
		})

		t.Run("priority", func(t *ftt.Test) {
			t.Run("negative", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Priority:        -1,
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("priority must be in"))
			})

			t.Run("excessive", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Priority:        256,
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("priority must be in"))
			})
		})

		t.Run("properties", func(t *ftt.Test) {
			t.Run("prohibited", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"buildbucket": {
								Kind: &structpb.Value_StringValue{},
							},
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.ErrLike("must not be specified"))
			})

			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": {
								Kind: &structpb.Value_StringValue{},
							},
						},
					},
					TemplateBuildId: 1,
				}
				err := validateSchedule(ctx, req, nil, nil)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("tags", func(t *ftt.Test) {
			req := &pb.ScheduleBuildRequest{
				Tags: []*pb.StringPair{
					{
						Key: "key:value",
					},
				},
				TemplateBuildId: 1,
			}
			err := validateSchedule(ctx, req, nil, nil)
			assert.Loosely(t, err, should.ErrLike("tags"))
		})

		t.Run("experiments", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Experiments: map[string]bool{
						bb.ExperimentBBAgent:    true,
						"cool.experiment_thing": true,
					},
				}
				assert.Loosely(t, validateSchedule(ctx, req, stringset.NewFromSlice(bb.ExperimentBBAgent), nil), should.BeNil)
			})

			t.Run("bad name", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Experiments: map[string]bool{
						"bad name": true,
					},
				}
				assert.Loosely(t, validateSchedule(ctx, req, nil, nil), should.ErrLike("does not match"))
			})

			t.Run("bad reserved", func(t *ftt.Test) {
				req := &pb.ScheduleBuildRequest{
					TemplateBuildId: 1,
					Experiments: map[string]bool{
						"luci.use_ralms": true,
					},
				}
				assert.Loosely(t, validateSchedule(ctx, req, nil, nil), should.ErrLike("unknown experiment has reserved prefix"))
			})
		})
	})

	ftt.Run("setInfraAgent", t, func(t *ftt.Test) {
		t.Run("bbagent+userpackages", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Canary: true,
				Exe: &pb.Executable{
					CipdPackage: "exe",
					CipdVersion: "exe-version",
				},
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
				Input: &pb.Build_Input{
					Experiments: []string{"omit", "include"},
				},
			}
			cfg := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					BbagentPackage: &pb.SwarmingSettings_Package{
						PackageName:   "infra/tools/luci/bbagent/${platform}",
						Version:       "version",
						VersionCanary: "canary-version",
					},
					UserPackages: []*pb.SwarmingSettings_Package{
						{
							PackageName:   "include",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							Builders: &pb.BuilderPredicate{
								RegexExclude: []string{
									".*",
								},
							},
							PackageName:   "exclude",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							Builders: &pb.BuilderPredicate{
								Regex: []string{
									".*",
								},
							},
							PackageName:   "subdir",
							Subdir:        "subdir",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							PackageName:         "include_experiment",
							Version:             "version",
							IncludeOnExperiment: []string{"include"},
						},
						{
							PackageName:         "not_include_experiment",
							Version:             "version",
							IncludeOnExperiment: []string{"not_include"},
						},
						{
							PackageName:      "omit_experiment",
							Version:          "version",
							OmitOnExperiment: []string{"omit"},
						},
					},
				},
				Cipd: &pb.CipdSettings{
					Server: "cipd server",
					Source: &pb.CipdSettings_Source{
						PackageName:   "the/offical/cipd/package/${platform}",
						Version:       "1",
						VersionCanary: "1canary",
					},
				},
			}
			err := setInfraAgent(b, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Infra.Buildbucket.Agent, should.Resemble(&pb.BuildInfra_Buildbucket_Agent{
				Source: &pb.BuildInfra_Buildbucket_Agent_Source{
					DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
						Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
							Package: "infra/tools/luci/bbagent/${platform}",
							Version: "canary-version",
							Server:  "cipd server",
						},
					},
				},
				Input: &pb.BuildInfra_Buildbucket_Agent_Input{
					CipdSource: map[string]*pb.InputDataRef{
						"cipd": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{
											Package: "the/offical/cipd/package/${platform}",
											Version: "1canary",
										},
									},
								},
							},
							OnPath: []string{"cipd", "cipd/bin"},
						},
					},
					Data: map[string]*pb.InputDataRef{
						"cipd_bin_packages": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{Package: "include", Version: "canary-version"},
										{Package: "include_experiment", Version: "version"},
									},
								},
							},
							OnPath: []string{"cipd_bin_packages", "cipd_bin_packages/bin"},
						},
						"cipd_bin_packages/subdir": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{Package: "subdir", Version: "canary-version"},
									},
								},
							},
							OnPath: []string{"cipd_bin_packages/subdir", "cipd_bin_packages/subdir/bin"},
						},
						"kitchen-checkout": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{Package: "exe", Version: "exe-version"},
									},
								},
							},
						},
					},
				},
				Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
					"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
				},
				CipdClientCache: &pb.CacheEntry{
					Name: "cipd_client_c3bb9331ecf2d9dfe25df9012569bcc1278974c87ea33a56b2f4aa2761078578",
					Path: "cipd_client",
				},
				CipdPackagesCache: &pb.CacheEntry{
					Name: "cipd_cache_e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Path: "cipd_cache",
				},
			}))
		})

		t.Run("bad bbagent cfg", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Exe: &pb.Executable{
					CipdPackage: "exe",
					CipdVersion: "exe-version",
				},
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
			}
			cfg := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					BbagentPackage: &pb.SwarmingSettings_Package{
						PackageName: "infra/tools/luci/bbagent/${bad}",
						Version:     "bbagent-version",
					},
				},
				Cipd: &pb.CipdSettings{
					Server: "cipd server",
					Source: &pb.CipdSettings_Source{
						PackageName: "the/offical/cipd/package/${platform}",
						Version:     "1",
					},
				},
			}
			err := setInfraAgent(b, cfg)
			assert.Loosely(t, err, should.ErrLike("bad settings: bbagent package name must end with '/${platform}'"))
			assert.Loosely(t, b.Infra.Buildbucket.Agent.Source, should.BeNil)
		})

		t.Run("empty settings", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
			}
			err := setInfraAgent(b, &pb.SettingsCfg{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Infra.Buildbucket.Agent.Source, should.BeNil)
			assert.Loosely(t, b.Infra.Buildbucket.Agent.Input.Data, should.BeEmpty)
		})

		t.Run("bbagent alternative", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Canary: true,
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
				Input: &pb.Build_Input{
					Experiments: []string{"omit", "include"},
				},
			}
			t.Run("cannot decide bbagent", func(t *ftt.Test) {
				cfg := &pb.SettingsCfg{
					Swarming: &pb.SwarmingSettings{
						BbagentPackage: &pb.SwarmingSettings_Package{
							PackageName:   "infra/tools/luci/bbagent/${platform}",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						AlternativeAgentPackages: []*pb.SwarmingSettings_Package{
							{
								PackageName:         "bbagent_alternative/${platform}",
								Version:             "version",
								IncludeOnExperiment: []string{"include"},
							},
							{
								PackageName:         "bbagent_alternative_2/${platform}",
								Version:             "version",
								IncludeOnExperiment: []string{"include"},
							},
						},
					},
					Cipd: &pb.CipdSettings{
						Server: "cipd server",
					},
				}
				err := setInfraAgent(b, cfg)
				assert.Loosely(t, err, should.ErrLike("cannot decide buildbucket agent source"))
			})
			t.Run("pass", func(t *ftt.Test) {
				cfg := &pb.SettingsCfg{
					Swarming: &pb.SwarmingSettings{
						BbagentPackage: &pb.SwarmingSettings_Package{
							PackageName:   "infra/tools/luci/bbagent/${platform}",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						AlternativeAgentPackages: []*pb.SwarmingSettings_Package{
							{
								PackageName:         "bbagent_alternative/${platform}",
								Version:             "version",
								IncludeOnExperiment: []string{"include"},
							},
						},
					},
					Cipd: &pb.CipdSettings{
						Server: "cipd server",
						Source: &pb.CipdSettings_Source{
							PackageName: "the/offical/cipd/package/${platform}",
							Version:     "1",
						},
					},
				}
				err := setInfraAgent(b, cfg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.Infra.Buildbucket.Agent, should.Resemble(&pb.BuildInfra_Buildbucket_Agent{
					Source: &pb.BuildInfra_Buildbucket_Agent_Source{
						DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
							Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
								Package: "bbagent_alternative/${platform}",
								Version: "version",
								Server:  "cipd server",
							},
						},
					},
					Input: &pb.BuildInfra_Buildbucket_Agent_Input{
						CipdSource: map[string]*pb.InputDataRef{
							"cipd": {
								DataType: &pb.InputDataRef_Cipd{
									Cipd: &pb.InputDataRef_CIPD{
										Server: "cipd server",
										Specs: []*pb.InputDataRef_CIPD_PkgSpec{
											{
												Package: "the/offical/cipd/package/${platform}",
												Version: "1",
											},
										},
									},
								},
								OnPath: []string{"cipd", "cipd/bin"},
							},
						},
					},
					Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
						"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					},
					CipdClientCache: &pb.CacheEntry{
						Name: "cipd_client_6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
						Path: "cipd_client",
					},
				}))
			})
		})

		t.Run("bbagent_utilility_packages", func(t *ftt.Test) {
			b := &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Canary: true,
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "app.appspot.com",
					},
				},
				Input: &pb.Build_Input{
					Experiments: []string{"omit", "include"},
				},
			}
			cfg := &pb.SettingsCfg{
				Swarming: &pb.SwarmingSettings{
					BbagentPackage: &pb.SwarmingSettings_Package{
						PackageName:   "infra/tools/luci/bbagent/${platform}",
						Version:       "version",
						VersionCanary: "canary-version",
					},
					BbagentUtilityPackages: []*pb.SwarmingSettings_Package{
						{
							PackageName:   "include",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							Builders: &pb.BuilderPredicate{
								RegexExclude: []string{
									".*",
								},
							},
							PackageName:   "exclude",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							Builders: &pb.BuilderPredicate{
								Regex: []string{
									".*",
								},
							},
							PackageName:   "subdir",
							Subdir:        "subdir",
							Version:       "version",
							VersionCanary: "canary-version",
						},
						{
							PackageName:         "include_experiment",
							Version:             "version",
							IncludeOnExperiment: []string{"include"},
						},
						{
							PackageName:         "not_include_experiment",
							Version:             "version",
							IncludeOnExperiment: []string{"not_include"},
						},
						{
							PackageName:      "omit_experiment",
							Version:          "version",
							OmitOnExperiment: []string{"omit"},
						},
					},
				},
				Cipd: &pb.CipdSettings{
					Server: "cipd server",
					Source: &pb.CipdSettings_Source{
						PackageName: "the/offical/cipd/package/${platform}",
						Version:     "1",
					},
				},
			}
			err := setInfraAgent(b, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Infra.Buildbucket.Agent, should.Resemble(&pb.BuildInfra_Buildbucket_Agent{
				Source: &pb.BuildInfra_Buildbucket_Agent_Source{
					DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
						Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
							Package: "infra/tools/luci/bbagent/${platform}",
							Version: "canary-version",
							Server:  "cipd server",
						},
					},
				},
				Input: &pb.BuildInfra_Buildbucket_Agent_Input{
					CipdSource: map[string]*pb.InputDataRef{
						"cipd": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{
											Package: "the/offical/cipd/package/${platform}",
											Version: "1",
										},
									},
								},
							},
							OnPath: []string{"cipd", "cipd/bin"},
						},
					},
					Data: map[string]*pb.InputDataRef{
						"bbagent_utility_packages": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{Package: "include", Version: "canary-version"},
										{Package: "include_experiment", Version: "version"},
									},
								},
							},
							OnPath: []string{"bbagent_utility_packages", "bbagent_utility_packages/bin"},
						},
						"bbagent_utility_packages/subdir": {
							DataType: &pb.InputDataRef_Cipd{
								Cipd: &pb.InputDataRef_CIPD{
									Server: "cipd server",
									Specs: []*pb.InputDataRef_CIPD_PkgSpec{
										{Package: "subdir", Version: "canary-version"},
									},
								},
							},
							OnPath: []string{"bbagent_utility_packages/subdir", "bbagent_utility_packages/subdir/bin"},
						},
					},
				},
				Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
					"kitchen-checkout":                pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
					"bbagent_utility_packages":        pb.BuildInfra_Buildbucket_Agent_PURPOSE_BBAGENT_UTILITY,
					"bbagent_utility_packages/subdir": pb.BuildInfra_Buildbucket_Agent_PURPOSE_BBAGENT_UTILITY,
				},
				CipdClientCache: &pb.CacheEntry{
					Name: "cipd_client_6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b",
					Path: "cipd_client",
				},
				CipdPackagesCache: &pb.CacheEntry{
					Name: "cipd_cache_e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					Path: "cipd_cache",
				},
			}))
		})
	})
}

func sortTasksByClassName(tasks tqtesting.TaskList) {
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Class < tasks[j].Class
	})
}
