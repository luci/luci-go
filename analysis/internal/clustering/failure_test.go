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

package clustering

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestFailure(t *testing.T) {
	ftt.Run(`Failure`, t, func(t *ftt.Test) {
		t.Run(`Nil failure`, func(t *ftt.Test) {
			var f *Failure
			assert.Loosely(t, f.Kind(), should.BeEmpty)
			assert.Loosely(t, f.GetErrorMessages(), should.BeNil)
			assert.Loosely(t, f.GetErrorTraces(), should.BeNil)
		})

		t.Run(`Nil Reason`, func(t *ftt.Test) {
			f := &Failure{
				TestID: "ninja://test",
			}
			assert.Loosely(t, f.Kind(), should.BeEmpty)
			assert.Loosely(t, f.GetErrorMessages(), should.BeNil)
			assert.Loosely(t, f.GetErrorTraces(), should.BeNil)
		})

		t.Run(`Empty Reason`, func(t *ftt.Test) {
			f := &Failure{
				TestID: "ninja://test",
				Reason: &pb.FailureReason{},
			}
			assert.Loosely(t, f.Kind(), should.BeEmpty)
			assert.Loosely(t, f.GetErrorMessages(), should.BeNil)
			assert.Loosely(t, f.GetErrorTraces(), should.BeNil)
		})

		t.Run(`Populated Reason and Caching`, func(t *ftt.Test) {
			f := &Failure{
				TestID: "ninja://test",
				Reason: &pb.FailureReason{
					Kind: pb.FailureReason_CRASH,
					Errors: []*pb.FailureReason_Error{
						{Message: "msg 1", Trace: "trace 1"},
						{Message: "msg 2", Trace: "trace 2"},
					},
				},
			}

			assert.Loosely(t, f.Kind(), should.Equal("CRASH"))

			msgs1 := f.GetErrorMessages()
			assert.Loosely(t, msgs1, should.Match([]string{"msg 1", "msg 2"}))

			traces1 := f.GetErrorTraces()
			assert.Loosely(t, traces1, should.Match([]string{"trace 1", "trace 2"}))

			// Subsequent calls return the identical slice references (cached).
			msgs2 := f.GetErrorMessages()
			assert.Loosely(t, &msgs1[0], should.Equal(&msgs2[0]))

			traces2 := f.GetErrorTraces()
			assert.Loosely(t, &traces1[0], should.Equal(&traces2[0]))
		})

		t.Run(`Kinds`, func(t *ftt.Test) {
			f := &Failure{Reason: &pb.FailureReason{Kind: pb.FailureReason_ORDINARY}}
			assert.Loosely(t, f.Kind(), should.Equal("ORDINARY"))

			f = &Failure{Reason: &pb.FailureReason{Kind: pb.FailureReason_TIMEOUT}}
			assert.Loosely(t, f.Kind(), should.Equal("TIMEOUT"))

			f = &Failure{Reason: &pb.FailureReason{Kind: pb.FailureReason_KIND_UNSPECIFIED}}
			assert.Loosely(t, f.Kind(), should.BeEmpty)
		})
	})
}
