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
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

func TestBQGroups(t *testing.T) {
	t.Parallel()

	ftt.Run("expandGroups works", t, func(t *ftt.Test) {
		ctx := context.Background()
		testRev := int64(1000)
		testTime := timestamppb.New(
			time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC))
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
						"group-missing",
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

		expected := []*bqpb.GroupRow{
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
				Subgroups: []string{
					"group-d",
					"group-missing",
				},
				DirectMembers: []string{
					"user:a1@example.com",
					"user:a2@example.com",
					"user:a3@example.com",
				},
				AuthdbRev:  testRev,
				ExportedAt: testTime,
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
				Subgroups: []string{
					"group-c",
					"group-d",
				},
				DirectMembers: []string{
					"user:b@example.com",
				},
				AuthdbRev:  testRev,
				ExportedAt: testTime,
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
				Subgroups: []string{
					"group-d",
				},
				DirectMembers: []string{
					"user:c1@example.com",
					"user:c2@example.com",
				},
				AuthdbRev:  testRev,
				ExportedAt: testTime,
			},
			{
				Name: "group-d",
				Members: []string{
					"user:d@test.com",
				},
				Globs: []string{
					"user:d*@example.com",
				},
				DirectMembers: []string{
					"user:d@test.com",
				},
				AuthdbRev:  testRev,
				ExportedAt: testTime,
			},
			{
				Name:       "group-missing",
				Owners:     model.AdminGroup,
				Missing:    true,
				AuthdbRev:  testRev,
				ExportedAt: testTime,
			},
		}

		actual, err := expandGroups(ctx, testAuthDB, testRev, testTime)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(expected))
	})
}
