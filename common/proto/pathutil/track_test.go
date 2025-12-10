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
	"testing"

	"go.chromium.org/luci/common/proto/pathutil"
	"go.chromium.org/luci/common/testing/truth/assert"
)

func TestTrackMsg(t *testing.T) {
	t.Run(`TrackRequiredMsg`, func(t *testing.T) {
		trk := testMessageTrackerFactory.New(4)
		m := &TestMessage{}
		for range pathutil.TrackRequiredMsg(trk, "msg", m.Msg) {
			trk.Err("should not reach")
		}

		assert.ErrIsLike(t, trk.Error(),
			`(pathutil_test.TestMessage).msg: required`)
	})

	t.Run(`TrackOptionalMsg`, func(t *testing.T) {
		trk := testMessageTrackerFactory.New(4)
		m := &TestMessage{}
		for range pathutil.TrackOptionalMsg(trk, "msg", m.Msg) {
			trk.Err("should not reach")
		}

		assert.NoErr(t, trk.Error())
	})
}
