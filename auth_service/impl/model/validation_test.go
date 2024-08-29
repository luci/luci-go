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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("Validation works", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("empty AuthDB", func(t *ftt.Test) {
			assert.Loosely(t, validateAuthDB(ctx, &protocol.AuthDB{}), should.BeNil)
		})

		t.Run("bad group name", func(t *ftt.Test) {
			db := &protocol.AuthDB{
				Groups: []*protocol.AuthGroup{
					{
						Name:       "Not-a-valid-name!",
						CreatedBy:  "user:somebody@example.com",
						ModifiedBy: "user:somebody@example.com",
					},
				},
			}
			assert.Loosely(t, validateAuthDB(ctx, db), should.ErrLike(ErrInvalidName))
		})

		t.Run("bad creator", func(t *ftt.Test) {
			db := &protocol.AuthDB{
				Groups: []*protocol.AuthGroup{
					{
						Name:       "test-group",
						ModifiedBy: "user:somebody@example.com",
					},
				},
			}
			assert.Loosely(t, validateAuthDB(ctx, db), should.ErrLike(ErrInvalidIdentity))
		})

		t.Run("bad modifier", func(t *ftt.Test) {
			db := &protocol.AuthDB{
				Groups: []*protocol.AuthGroup{
					{
						Name:      "test-group",
						CreatedBy: "user:somebody@example.com",
					},
				},
			}
			assert.Loosely(t, validateAuthDB(ctx, db), should.ErrLike(ErrInvalidIdentity))
		})

		t.Run("unknown group", func(t *ftt.Test) {
			db := &protocol.AuthDB{
				Groups: []*protocol.AuthGroup{
					{
						Name:       "test-group",
						CreatedBy:  "user:somebody@example.com",
						ModifiedBy: "user:somebody@example.com",
						Nested:     []string{"subgroup"},
					},
				},
			}
			assert.Loosely(t, validateAuthDB(ctx, db), should.ErrLike("unknown nested group"))
		})

		t.Run("valid non-empty", func(t *ftt.Test) {
			db := &protocol.AuthDB{
				Groups: []*protocol.AuthGroup{
					{
						Name:       "test-group-a",
						CreatedBy:  "user:somebody@example.com",
						ModifiedBy: "user:somebody@example.com",
						Members:    []string{"user:a@example.com"},
						Nested:     []string{"test-group-b", "external/test-external-group"},
					},
					{
						Name:       "test-group-b",
						CreatedBy:  "user:somebody@example.com",
						ModifiedBy: "user:somebody@example.com",
						Members:    []string{"user:b@example.com"},
						Globs:      []string{"user:*body@example.com"},
					},
					{
						Name:       "external/test-external-group",
						CreatedBy:  "user:somebody@example.com",
						ModifiedBy: "user:somebody@example.com",
						Members:    []string{"user:e@external.domain"},
					},
				},
				IpWhitelists: []*protocol.AuthIPWhitelist{
					{Name: "IP allowlist"},
				},
			}
			assert.Loosely(t, validateAuthDB(ctx, db), should.BeNil)
		})
	})
}
