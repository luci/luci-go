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

package bqexporter

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestGenerateArtifactBQRow(t *testing.T) {
	t.Parallel()

	ftt.Run("GenerateBQRow", t, func(t *ftt.Test) {
		input := &textArtifactRowInput{
			exported: &pb.Invocation{
				Name:       "invocations/exported",
				CreateTime: pbutil.MustTimestampProto(testclock.TestRecentTimeUTC),
				Realm:      "testproject:testrealm",
			},
			parent: &pb.Invocation{
				Name: "invocations/parent",
			},
			a: &pb.Artifact{
				Name:      "invocations/a/tests/ninja:%2F%2Fchrome%2Ftest:foo_tests%2FBarTest.DoBaz/results/result5/artifacts/a",
				SizeBytes: 2e7,
			},
			shardID: 0,
			content: "deadbeef",
		}
		row := input.row()
		actual, ok := row.(*bqpb.TextArtifactRowLegacy)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, actual.Content, should.Match(input.content))

		assert.Loosely(t, input.id(), should.Match([]byte(fmt.Sprintf("%s/%d", input.a.Name, input.shardID))))
	})
}
