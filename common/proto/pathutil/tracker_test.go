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

package pathutil_test

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto/pathutil"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
)

func shouldHaveErrors(errs ...string) comparison.Func[*pathutil.Tracker] {
	return func(t *pathutil.Tracker) *failure.Summary {
		errsSet := stringset.NewFromSlice(errs...)

		sum := comparison.NewSummaryBuilder("shouldHaveErrors", t)

		for _, err := range t.Error().(pathutil.Errors).Clone(pathutil.WithoutRoot) {
			errS := err.Error()
			if !errsSet.Del(errS) {
				sum.AddFindingf("unexpected", "%q", errS)
			}
		}
		for _, wanted := range errsSet.ToSortedSlice() {
			sum.AddFindingf("missing", "%q", wanted)
		}

		if len(sum.Findings) == 0 {
			return nil
		}
		return sum.Summary
	}
}

func TestDeepnessCheck(t *testing.T) {
	t.Run(`no depth check`, func(t *testing.T) {
		sample := &TestMessage{
			Msg: &TestMessage{
				Msg: &TestMessage{},
			},
		}

		track := testMessageTrackerFactory.New(0)
		for range pathutil.TrackRequiredMsg(track, "msg", sample.GetMsg()) {
			for range pathutil.TrackRequiredMsg(track, "msg", sample.GetMsg()) {
				track.Err("should see this")
			}
		}

		assert.That(t, track, shouldHaveErrors(
			".msg.msg: should see this",
		))
	})

	t.Run(`overrun`, func(t *testing.T) {
		sample := &TestMessage{
			Msg: &TestMessage{
				Msg: &TestMessage{},
			},
		}

		track := testMessageTrackerFactory.New(1)
		for range pathutil.TrackRequiredMsg(track, "msg", sample.GetMsg()) {
			for range pathutil.TrackRequiredMsg(track, "msg", sample.GetMsg()) {
				track.Err("should not see this")
			}
		}

		assert.That(t, track, shouldHaveErrors(
			".msg: exceeds maximum depth 1",
		))
	})
}

func TestErr(t *testing.T) {
	track := testMessageTrackerFactory.New(1)
	track.Err("something")

	sentinel := fmt.Errorf("sentinel")
	track.Err("%w", sentinel)

	track.FieldErr("scalar", "scalar %s", "error")

	assert.That(t, track, shouldHaveErrors(
		": something",
		": sentinel",
		".scalar: scalar error",
	))

	assert.That(t, track.Error().(pathutil.Errors)[1].Wrapped,
		should.Equal(sentinel))
}

func TestNoSuchField(t *testing.T) {
	track := testMessageTrackerFactory.New(10)
	assert.That(t, func() {
		track.FieldErr("not real", "this does not exist")
	}, should.PanicLikeString(
		`pathutil.Tracker: field "not real" in message (pathutil_test.TestMessage) does not exist`,
	))
	assert.Loosely(t, track.Error(), should.BeNil)
}
