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

package verdict

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestVerdictGroupStatus(t *testing.T) {
	t.Parallel()

	ftt.Run(`VerdictGroup status`, t, func(t *ftt.Test) {
		g := &VerdictGroup{}

		t.Run(`empty results`, func(t *ftt.Test) {
			assert.Loosely(t, g.Status(), should.Equal("STATUS_UNSPECIFIED"))
		})

		t.Run(`all passed`, func(t *ftt.Test) {
			g.Results = []*pb.TestResult{
				{StatusV2: pb.TestResult_PASSED},
				{StatusV2: pb.TestResult_PASSED},
			}
			assert.Loosely(t, g.Status(), should.Equal("PASSED"))
		})

		t.Run(`all failed`, func(t *ftt.Test) {
			g.Results = []*pb.TestResult{
				{StatusV2: pb.TestResult_FAILED},
				{StatusV2: pb.TestResult_FAILED},
			}
			assert.Loosely(t, g.Status(), should.Equal("FAILED"))
		})

		t.Run(`flaky`, func(t *ftt.Test) {
			g.Results = []*pb.TestResult{
				{StatusV2: pb.TestResult_FAILED},
				{StatusV2: pb.TestResult_PASSED},
			}
			assert.Loosely(t, g.Status(), should.Equal("FLAKY"))
		})

		t.Run(`skipped and failed`, func(t *ftt.Test) {
			g.Results = []*pb.TestResult{
				{StatusV2: pb.TestResult_SKIPPED},
				{StatusV2: pb.TestResult_FAILED},
			}
			assert.Loosely(t, g.Status(), should.Equal("FAILED"))
		})

		t.Run(`only skipped`, func(t *ftt.Test) {
			g.Results = []*pb.TestResult{
				{StatusV2: pb.TestResult_SKIPPED},
			}
			assert.Loosely(t, g.Status(), should.Equal("SKIPPED"))
		})

		t.Run(`execution errored`, func(t *ftt.Test) {
			g.Results = []*pb.TestResult{
				{StatusV2: pb.TestResult_EXECUTION_ERRORED},
			}
			assert.Loosely(t, g.Status(), should.Equal("EXECUTION_ERRORED"))
		})
	})
}
