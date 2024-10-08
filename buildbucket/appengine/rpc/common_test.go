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
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/bqlog"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

func installTestSecret(ctx context.Context) context.Context {
	store := &testsecrets.Store{
		Secrets: map[string]secrets.Secret{
			"key": {Active: []byte("stuff")},
		},
	}
	ctx = secrets.Use(ctx, store)
	return secrets.GeneratePrimaryTinkAEADForTest(ctx)
}

func TestLogToBQ(t *testing.T) {
	t.Parallel()

	ftt.Run("logToBQ", t, func(c *ftt.Test) {
		b := &bqlog.Bundler{
			CloudProject: "project",
			Dataset:      "dataset",
		}
		ctx := withBundler(context.Background(), b)
		b.RegisterSink(bqlog.Sink{
			Prototype: &pb.PRPCRequestLog{},
			Table:     "table",
		})
		b.Start(ctx, &bqlog.FakeBigQueryWriter{
			Send: func(req *storagepb.AppendRowsRequest) error {
				rows := req.GetProtoRows().GetRows().GetSerializedRows()
				// TODO(crbug/1250459): Check that rows being sent to BQ look correct.
				assert.Loosely(c, len(rows), should.Equal(1))
				return nil
			},
		})
		defer b.Shutdown(ctx)

		logToBQ(ctx, "id", "parent", "method")
	})
}

func TestValidateTags(t *testing.T) {
	t.Parallel()

	ftt.Run("validate build set", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			// test gitiles format
			gitiles := fmt.Sprintf("commit/gitiles/chromium.googlesource.com/chromium/src/+/%s", strings.Repeat("a", 40))
			assert.Loosely(t, validateBuildSet(gitiles), should.BeNil)
			// test gerrit format
			assert.Loosely(t, validateBuildSet("patch/gerrit/chromium-review.googlesource.com/123/456"), should.BeNil)
			// test user format
			assert.Loosely(t, validateBuildSet("myformat/x"), should.BeNil)
		})
		t.Run("invalid", func(t *ftt.Test) {
			gitiles := fmt.Sprintf("commit/gitiles/chromium.googlesource.com/a/chromium/src/+/%s", strings.Repeat("a", 40))
			assert.Loosely(t, validateBuildSet(gitiles), should.ErrLike(`gitiles project must not start with "a/"`))
			gitiles = fmt.Sprintf("commit/gitiles/chromium.googlesource.com/chromium/src.git/+/%s", strings.Repeat("a", 40))
			assert.Loosely(t, validateBuildSet(gitiles), should.ErrLike(`gitiles project must not end with ".git"`))

			assert.Loosely(t, validateBuildSet("patch/gerrit/chromium-review.googlesource.com/aa/bb"), should.ErrLike(`does not match regex "^patch/gerrit/([^/]+)/(\d+)/(\d+)$"`))
			assert.Loosely(t, validateBuildSet(strings.Repeat("a", 2000)), should.ErrLike("buildset tag is too long"))
		})
	})

	ftt.Run("validate tags", t, func(t *ftt.Test) {
		t.Run("invalid", func(t *ftt.Test) {
			// in general
			assert.Loosely(t, validateTags([]*pb.StringPair{{Key: "k:1", Value: "v"}}, TagNew), should.ErrLike("cannot have a colon"))

			// build address
			assert.Loosely(t, validateTags([]*pb.StringPair{{Key: "build_address", Value: "v"}}, TagNew), should.ErrLike(`tag "build_address" is reserved`))
			assert.Loosely(t, validateTags([]*pb.StringPair{{Key: "build_address", Value: "v"}}, TagAppend), should.ErrLike(`cannot be added to an existing build`))

			// buildset
			assert.Loosely(t, validateTags([]*pb.StringPair{{Key: "buildset", Value: "patch/gerrit/foo"}}, TagNew), should.ErrLike(`does not match regex "^patch/gerrit/([^/]+)/(\d+)/(\d+)$"`))

			gitiles1 := fmt.Sprintf("commit/gitiles/chromium.googlesource.com/chromium/src/+/%s", strings.Repeat("a", 40))
			gitiles2 := fmt.Sprintf("commit/gitiles/chromium.googlesource.com/chromium/src/+/%s", strings.Repeat("b", 40))
			assert.Loosely(t, validateTags([]*pb.StringPair{
				{Key: "buildset", Value: gitiles1},
				{Key: "buildset", Value: gitiles2},
			}, TagNew),
				should.BeNil)
			assert.Loosely(t, validateTags([]*pb.StringPair{
				{Key: "buildset", Value: gitiles1},
				{Key: "buildset", Value: gitiles1},
			}, TagNew),
				should.BeNil)

			// builder
			assert.Loosely(t, validateTags([]*pb.StringPair{
				{Key: "builder", Value: "1"},
				{Key: "builder", Value: "2"},
			}, TagNew),
				should.ErrLike(
					`tag "builder:2" conflicts with tag "builder:1"`))
			assert.Loosely(t, validateTags([]*pb.StringPair{
				{Key: "builder", Value: "1"},
				{Key: "builder", Value: "1"},
			}, TagNew),
				should.BeNil)
			assert.Loosely(t, validateTags([]*pb.StringPair{{Key: "builder", Value: "v"}}, TagAppend), should.ErrLike("cannot be added to an existing build"))
		})
	})

	ftt.Run("validate summary_markdown", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			assert.Loosely(t, validateSummaryMarkdown("[this](http://example.org) is a link"), should.BeNil)
		})

		t.Run("too big", func(t *ftt.Test) {
			assert.Loosely(t, validateSummaryMarkdown(strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength)), should.ErrLike("too big to accept"))
		})
	})

	ftt.Run("validateCommit", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			err := validateCommit(nil)
			assert.Loosely(t, err, should.ErrLike("host is required"))
		})

		t.Run("empty", func(t *ftt.Test) {
			cm := &pb.GitilesCommit{}
			err := validateCommit(cm)
			assert.Loosely(t, err, should.ErrLike("host is required"))
		})

		t.Run("project", func(t *ftt.Test) {
			cm := &pb.GitilesCommit{
				Host: "host",
			}
			err := validateCommit(cm)
			assert.Loosely(t, err, should.ErrLike("project is required"))
		})

		t.Run("id", func(t *ftt.Test) {
			t.Run("invalid ID", func(t *ftt.Test) {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "id",
				}
				err := validateCommit(cm)
				// sha1
				assert.Loosely(t, err, should.ErrLike("id must match"))
			})

			t.Run("position", func(t *ftt.Test) {
				cm := &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Id:       "id",
					Position: 1,
				}
				err := validateCommit(cm)
				assert.Loosely(t, err, should.ErrLike("position requires ref"))
			})
		})

		t.Run("ref", func(t *ftt.Test) {
			t.Run("invalid ref", func(t *ftt.Test) {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Ref:     "ref",
				}
				err := validateCommit(cm)
				assert.Loosely(t, err, should.ErrLike("ref must match"))
			})

			t.Run("valid, but w/ invalid ID", func(t *ftt.Test) {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Ref:     "refs/r1",
					Id:      "id",
				}
				err := validateCommit(cm)
				assert.Loosely(t, err, should.ErrLike("id must match"))
			})
		})

		t.Run("neither ID nor ref", func(t *ftt.Test) {
			cm := &pb.GitilesCommit{
				Host:    "host",
				Project: "project",
			}
			err := validateCommit(cm)
			assert.Loosely(t, err, should.ErrLike("one of"))
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("id", func(t *ftt.Test) {
				cm := &pb.GitilesCommit{
					Host:    "host",
					Project: "project",
					Id:      "1234567890123456789012345678901234567890",
				}
				err := validateCommit(cm)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("ref", func(t *ftt.Test) {
				cm := &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Ref:      "refs/ref",
					Position: 1,
				}
				err := validateCommit(cm)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})

	ftt.Run("validateCommitWithRef", t, func(t *ftt.Test) {
		t.Run("nil", func(t *ftt.Test) {
			assert.Loosely(t, validateCommitWithRef(nil), should.ErrLike("ref is required"))
		})

		t.Run("empty", func(t *ftt.Test) {
			assert.Loosely(t, validateCommitWithRef(&pb.GitilesCommit{}), should.ErrLike("ref is required"))
		})

		t.Run("with id", func(t *ftt.Test) {
			cm := &pb.GitilesCommit{
				Host:    "host",
				Project: "project",
				Id:      "id",
			}
			assert.Loosely(t, validateCommitWithRef(cm), should.ErrLike("ref is required"))
		})

		t.Run("with ref", func(t *ftt.Test) {
			cm := &pb.GitilesCommit{
				Host:     "host",
				Project:  "project",
				Ref:      "refs/",
				Position: 1,
			}
			assert.Loosely(t, validateCommitWithRef(cm), should.BeNil)
		})
	})
}

func TestValidateBuildToken(t *testing.T) {
	t.Parallel()

	ftt.Run("validateBuildToken", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, time.Unix(1444945245, 0))
		ctx = installTestSecret(ctx)

		tk1, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_BUILD)
		tk2, _ := buildtoken.GenerateToken(ctx, 2, pb.TokenBody_BUILD)
		build := &model.Build{
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
			UpdateToken: tk1,
		}
		assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)

		t.Run("Works", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk1))
			_, err := validateToken(ctx, 1, pb.TokenBody_BUILD)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Fails", func(t *ftt.Test) {
			t.Run("if unmatched", func(t *ftt.Test) {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk2))
				_, err := validateToken(ctx, 1, pb.TokenBody_BUILD)
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("if missing", func(t *ftt.Test) {
				_, err := validateToken(ctx, 1, pb.TokenBody_BUILD)
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
	})
}

func TestValidateBuildTaskToken(t *testing.T) {
	t.Parallel()

	ftt.Run("validateBuildTaskToken", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, time.Unix(1444945245, 0))
		ctx = installTestSecret(ctx)

		tk1, err := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_TASK)
		assert.Loosely(t, err, should.BeNil)
		tk2, err := buildtoken.GenerateToken(ctx, 2, pb.TokenBody_TASK)
		assert.Loosely(t, err, should.BeNil)

		build := &model.Build{
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
		}
		assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)

		t.Run("Works", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk1))
			_, err := validateToken(ctx, 1, pb.TokenBody_TASK)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Fails", func(t *ftt.Test) {
			t.Run("if unmatched", func(t *ftt.Test) {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk2))
				_, err := validateToken(ctx, 1, pb.TokenBody_TASK)
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("if missing", func(t *ftt.Test) {
				_, err := validateToken(ctx, 1, pb.TokenBody_TASK)
				assert.Loosely(t, err, should.NotBeNil)
			})
			t.Run("if wrong purpose", func(t *ftt.Test) {
				_, err := validateToken(ctx, 1, pb.TokenBody_BUILD)
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
	})
}
