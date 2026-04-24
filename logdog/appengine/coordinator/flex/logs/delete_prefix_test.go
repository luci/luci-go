// Copyright 2026 The LUCI Authors.
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

package logs

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
)

func TestDeletePrefix(t *testing.T) {
	t.Parallel()

	setup := func(t testing.TB) (logdog.LogsServer, *ct.Environment, context.Context) {
		ctx, env := ct.Install(t.Context())
		env.AddProject(ctx, "project")

		assert.NoErr(t, ct.MakeStream(ctx, "project", "realm", "prefix/+/cool").Put(ctx))
		assert.NoErr(t, ct.MakeStream(ctx, "project", "realm", "prefix/+/cool/beans").Put(ctx))

		return New(), env, ctx
	}

	t.Run(`ok`, func(t *testing.T) {
		t.Parallel()

		srv, env, ctx := setup(t)
		env.ActAsPrefixDeleter("project", "realm")
		_, err := srv.DeletePrefix(ctx, &logdog.DeletePrefixRequest{
			Project: "project",
			Prefix:  "prefix",
		})
		assert.NoErr(t, err)

		assert.NoErr(t, coordinator.WithProjectNamespace(&ctx, "project"))
		lsq, err := coordinator.NewLogStreamQuery("prefix")
		assert.NoErr(t, err)

		var streams []*coordinator.LogStream
		assert.NoErr(t, lsq.Run(ctx, func(ls *coordinator.LogStream, cc ds.CursorCB) error {
			streams = append(streams, ls)
			return nil
		}))
		assert.Loosely(t, streams, should.BeEmpty)
	})

	t.Run(`ok_hash`, func(t *testing.T) {
		t.Parallel()

		srv, env, ctx := setup(t)
		env.ActAsPrefixDeleter("project", "realm")
		_, err := srv.DeletePrefix(ctx, &logdog.DeletePrefixRequest{
			Project: "project",
			Prefix:  string(coordinator.LogPrefixID("prefix")),
		})
		assert.NoErr(t, err)

		assert.NoErr(t, coordinator.WithProjectNamespace(&ctx, "project"))
		lsq, err := coordinator.NewLogStreamQuery("prefix")
		assert.NoErr(t, err)

		var streams []*coordinator.LogStream
		assert.NoErr(t, lsq.Run(ctx, func(ls *coordinator.LogStream, cc ds.CursorCB) error {
			streams = append(streams, ls)
			return nil
		}))
		assert.Loosely(t, streams, should.BeEmpty)
	})

	t.Run(`noexist`, func(t *testing.T) {
		t.Parallel()

		srv, _, ctx := setup(t)
		_, err := srv.DeletePrefix(ctx, &logdog.DeletePrefixRequest{
			Project: "project",
			Prefix:  "other-prefix",
		})
		assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
	})

	t.Run(`noperm`, func(t *testing.T) {
		t.Parallel()

		srv, env, ctx := setup(t)
		env.ActAsPrefixDeleter("project", "unrelated-realm")
		_, err := srv.DeletePrefix(ctx, &logdog.DeletePrefixRequest{
			Project: "project",
			Prefix:  "prefix",
		})
		assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})
}
