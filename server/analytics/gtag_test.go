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

package analytics

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateMeasurementID(t *testing.T) {
	ftt.Run("Validate measurement ID", t, func(t *ftt.Test) {
		assert.Loosely(t, rGA4Allowed.MatchString("G-YP2P6PXQDT"), should.BeTrue)
		assert.Loosely(t, rGA4Allowed.MatchString("UA-55762617-26"), should.BeFalse)
		assert.Loosely(t, rGA4Allowed.MatchString(""), should.BeFalse)
		assert.Loosely(t, rGA4Allowed.MatchString("G-"), should.BeFalse)
		assert.Loosely(t, rGA4Allowed.MatchString("G-yp2p6pxqdt"), should.BeFalse)
		assert.Loosely(t, rGA4Allowed.MatchString(" G-YP2P6PXQDT "), should.BeFalse)
	})
}
