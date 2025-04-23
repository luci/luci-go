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

package debugstack

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func recurse(N, skip int) string {
	if N == 0 {
		return Capture(skip).String()
	}
	return recurse(N-1, skip)
}

func countFrameKinds(t Trace) map[FrameKind]int {
	ret := map[FrameKind]int{}
	for _, frame := range t {
		ret[frame.Kind]++
	}
	return ret
}

func TestCaptureDeepStack(t *testing.T) {
	t.Parallel()

	parsed := ParseString(recurse(1000, 0))

	// Go, by default, keeps the top 50 and bottom 50 frames at the point that
	// debug.Stack() is called inside Capture. Capture then removes two
	// StackFrameKinds (debug.Stack and Capture itself), leaving 98
	// StackFrameKinds.
	//
	// Additional frames are:
	//   - GoroutineHeaderKind at the top
	//   - FramesElidedKind in the middle
	//   - CreatedByFrameKind at the bottom
	assert.Loosely(t, parsed, should.HaveLength(101))
	assert.Loosely(t, countFrameKinds(parsed), should.Match(map[FrameKind]int{
		GoroutineHeaderKind: 1,
		StackFrameKind:      98,
		FramesElidedKind:    1,
		CreatedByFrameKind:  1,
	}))
	assert.That(t, parsed[0].Kind, should.Equal(GoroutineHeaderKind))
	assert.That(t, parsed[1].Kind, should.Equal(StackFrameKind))
	assert.That(t, parsed[1].PkgName, should.Equal(
		"go.chromium.org/luci/common/runtime/debugstack",
	))
	assert.That(t, parsed[1].FuncName, should.Equal("recurse"))
	assert.That(t, parsed[len(parsed)-1].Kind, should.Equal(CreatedByFrameKind))
	assert.That(t, parsed[len(parsed)-1].PkgName, should.Equal("testing"))
	assert.That(t, parsed[len(parsed)-1].FuncName, should.Equal("(*T).Run"))
}
