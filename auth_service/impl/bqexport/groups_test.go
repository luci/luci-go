// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"
)

func TestGroupsServer(t *testing.T) {
	t.Parallel()

	ftt.Run("expandGroups works", t, func(t *ftt.Test) {
		ctx := context.Background()
		testAuthDB := &protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name: "group-b",
					Members: []string{
						"user:b@example.com",
					},
					Nested: []string{
						"group-c",
					},
				},
				{
					Name: "group-c",
					Members: []string{
						"user:c1@example.com", "user:c2@example.com",
					},
					Nested: []string{
						"group-d",
					},
				},
				{
					Name: "group-a",
					Members: []string{
						"user:a1@example.com", "user:a2@example.com", "user:a3@example.com",
					},
					Nested: []string{
						"group-d",
					},
				},
				{
					Name: "group-d",
					Members: []string{
						"user:d@test.com",
					},
					Globs: []string{
						"user:d*@example.com",
					},
				},
			},
		}

		expected := []*rpcpb.AuthGroup{
			{
				Name: "group-a",
				Members: []string{
					"user:a1@example.com",
					"user:a2@example.com",
					"user:a3@example.com",
					"user:d@test.com",
				},
				Globs: []string{
					"user:d*@example.com",
				},
				Nested: []string{
					"group-d",
				},
			},
			{
				Name: "group-b",
				Members: []string{
					"user:b@example.com",
					"user:c1@example.com",
					"user:c2@example.com",
					"user:d@test.com",
				},
				Globs: []string{
					"user:d*@example.com",
				},
				Nested: []string{
					"group-c",
					"group-d",
				},
			},
			{
				Name: "group-c",
				Members: []string{
					"user:c1@example.com",
					"user:c2@example.com",
					"user:d@test.com",
				},
				Globs: []string{
					"user:d*@example.com",
				},
				Nested: []string{
					"group-d",
				},
			},
			{
				Name: "group-d",
				Members: []string{
					"user:d@test.com",
				},
				Globs: []string{
					"user:d*@example.com",
				},
			},
		}

		actual, err := expandGroups(ctx, testAuthDB)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(expected))
	})
}
