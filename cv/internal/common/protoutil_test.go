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

package common

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPB2Time(t *testing.T) {
	t.Parallel()

	ftt.Run("RoundTrip", t, func(t *ftt.Test) {
		t.Run("Specified", func(t *ftt.Test) {
			ts := testclock.TestRecentTimeUTC
			pb := Time2PBNillable(ts)
			assert.Loosely(t, pb, should.Resemble(Time2PBNillable(ts)))
			assert.Loosely(t, pb, should.Resemble(timestamppb.New(ts)))
			assert.Loosely(t, PB2TimeNillable(pb), should.Equal(ts))
		})
		t.Run("Zero / nil", func(t *ftt.Test) {
			assert.Loosely(t, PB2TimeNillable(nil), should.Equal(time.Time{}))
			assert.Loosely(t, Time2PBNillable(time.Time{}), should.BeNil)
		})
	})
}
