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

package gerrit

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/servicecfg"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMakeClient(t *testing.T) {
	t.Parallel()

	Convey("factory.token works", t, func() {
		ctx := memory.Use(context.Background())
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		So(datastore.Put(ctx, &netrcToken{"first.example.com", "legacy-1"}), ShouldBeNil)
		err := servicecfg.SetTestMigrationConfig(ctx, &migrationpb.Settings{
			PssaMigration: &migrationpb.PSSAMigration{
				ProjectsBlocklist: []string{"force-legacy"},
			},
		})
		So(err, ShouldBeNil)
		f := newFactory()

		Convey("works", func() {
			Convey("forced legacy", func() {
				t, err := f.token(ctx, "first.example.com", "force-legacy")
				So(err, ShouldBeNil)
				So(t.AccessToken, ShouldEqual, "bGVnYWN5LTE=") // base64 of "legacy-1"
				So(t.TokenType, ShouldEqual, "Basic")
			})

			Convey("fallback to legacy", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, nil
				}
				t, err := f.token(ctx, "first.example.com", "not-migrated")
				So(err, ShouldBeNil)
				So(t.AccessToken, ShouldEqual, "bGVnYWN5LTE=") // base64 of "legacy-1"
				So(t.TokenType, ShouldEqual, "Basic")
			})

			Convey("project-scoped account", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return &auth.Token{Token: "modern-1", Expiry: epoch.Add(2 * time.Minute)}, nil
				}
				t, err := f.token(ctx, "first.example.com", "modern")
				So(err, ShouldBeNil)
				So(t.AccessToken, ShouldEqual, "modern-1")
				So(t.TokenType, ShouldEqual, "Bearer")
			})
		})

		Convey("not works", func() {
			Convey("no legacy host", func() {
				_, err := f.token(ctx, "second.example.com", "force-legacy")
				So(err, ShouldErrLike, "No legacy credentials")
			})

			Convey("modern errors out", func() {
				f.mockMintProjectToken = func(context.Context, auth.ProjectTokenParams) (*auth.Token, error) {
					return nil, errors.New("flake")
				}
				_, err := f.token(ctx, "first.example.com", "modern")
				So(err, ShouldErrLike, "flake")
			})
		})
	})
}
