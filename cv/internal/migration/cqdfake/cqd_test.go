// Copyright 2021 The LUCI Authors.
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

package cqdfake

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"

	bqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/migration"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCQDFake(t *testing.T) {
	t.Parallel()

	Convey("Smoke test for CQDFake", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		cqd := CQDFake{
			LUCIProject: "test",
			CV:          &migration.MigrationServer{},
		}
		cqd.SetCandidatesClbk(func() []*migrationpb.Run {
			return []*migrationpb.Run{
				{
					Attempt: &bqpb.Attempt{
						Key: "beef01",
					},
				},
			}
		})

		cqd.Start(ctx)
		defer cqd.Close()
	})
}
