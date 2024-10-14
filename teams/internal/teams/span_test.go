// Copyright 2024 The LUCI Authors.
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

package teams

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/teams/internal/testutil"
)

func TestValidation(t *testing.T) {
	ftt.Run("Validate", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			err := Validate(NewTeamBuilder().Build())
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("id", func(t *ftt.Test) {
			t.Run("must be specified", func(t *ftt.Test) {
				err := Validate(NewTeamBuilder().WithID("").Build())
				assert.Loosely(t, err, should.ErrLike("id: must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := Validate(NewTeamBuilder().WithID("INVALID").Build())
				assert.Loosely(t, err, should.ErrLike("id: expected format"))
			})
		})
	})
}

func TestStatusTable(t *testing.T) {
	ftt.Run("Create", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		team := NewTeamBuilder().Build()

		m, err := Create(team)
		assert.Loosely(t, err, should.BeNil)
		ts, err := span.Apply(ctx, []*spanner.Mutation{m})
		team.CreateTime = ts.UTC()

		assert.Loosely(t, err, should.BeNil)
		fetched, err := Read(span.Single(ctx), team.ID)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fetched, should.Resemble(team))
	})

	ftt.Run("Read", t, func(t *ftt.Test) {
		t.Run("Single", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)
			team, err := NewTeamBuilder().CreateInDB(ctx)
			assert.Loosely(t, err, should.BeNil)

			fetched, err := Read(span.Single(ctx), team.ID)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched, should.Resemble(team))
		})

		t.Run("Not exists", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)

			_, err := Read(span.Single(ctx), "123456")

			assert.Loosely(t, err, should.Equal(NotExistsErr))
		})
	})
}
