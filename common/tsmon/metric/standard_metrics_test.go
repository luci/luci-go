// Copyright 2016 The LUCI Authors.
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

package metric

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
)

func TestStandardMetrics(t *testing.T) {
	t.Parallel()

	durMillis := 5.0
	dur := time.Duration(durMillis * float64(time.Millisecond))

	ftt.Run("UpdatePresenceMetrics updates presenceMetric", t, func(t *ftt.Test) {
		c, m := tsmon.WithDummyInMemory(context.Background())
		registerCallbacks(c)

		assert.Loosely(t, tsmon.Flush(c), should.BeNil)

		assert.Loosely(t, len(m.Cells), should.Equal(1))
		assert.Loosely(t, len(m.Cells[0]), should.Equal(1))
		assert.Loosely(t, m.Cells[0][0].Name, should.Equal("presence/up"))
		assert.Loosely(t, m.Cells[0][0].Value, should.Equal(true))
	})

	ftt.Run("UpdateHTTPMetrics updates client metrics", t, func(t *ftt.Test) {
		c := makeContext()
		name, client := "test_name", "test_client"
		assert.Loosely(t, requestBytesMetric.Get(c, name, client), should.BeNil)
		assert.Loosely(t, responseBytesMetric.Get(c, name, client), should.BeNil)
		assert.Loosely(t, requestDurationsMetric.Get(c, name, client), should.BeNil)
		assert.Loosely(t, responseStatusMetric.Get(c, 200, name, client), should.BeZero)

		UpdateHTTPMetrics(c, name, client, 200, dur, 123, 321)

		assert.That(t, requestBytesMetric.Get(c, name, client).Sum(), should.Equal(123.0))
		assert.That(t, responseBytesMetric.Get(c, name, client).Sum(), should.Equal(321.0))
		assert.That(t, requestDurationsMetric.Get(c, name, client).Sum(), should.Equal(durMillis))
		assert.Loosely(t, responseStatusMetric.Get(c, 200, name, client), should.Equal(1))
	})

	ftt.Run("UpdateServerMetrics updates server metrics", t, func(t *ftt.Test) {
		c := makeContext()
		code, name, isRobot := 200, "test_client", false

		assert.Loosely(t, serverDurationsMetric.Get(c, code, name, isRobot), should.BeNil)
		assert.Loosely(t, serverRequestBytesMetric.Get(c, code, name, isRobot), should.BeNil)
		assert.Loosely(t, serverResponseBytesMetric.Get(c, code, name, isRobot), should.BeNil)
		assert.Loosely(t, serverResponseStatusMetric.Get(c, code, name, isRobot), should.BeZero)

		t.Run("for a robot user agent", func(t *ftt.Test) {
			isRobot = true
			userAgent := "I am a GoogleBot."

			UpdateServerMetrics(c, name, code, dur, 123, 321, userAgent)

			assert.That(t, serverDurationsMetric.Get(c, code, name, isRobot).Sum(), should.Equal(durMillis))
			assert.That(t, serverRequestBytesMetric.Get(c, code, name, isRobot).Sum(), should.Equal(123.0))
			assert.That(t, serverResponseBytesMetric.Get(c, code, name, isRobot).Sum(), should.Equal(321.0))
			assert.Loosely(t, serverResponseStatusMetric.Get(c, code, name, isRobot), should.Equal(1))
		})

		t.Run("for a non-robot user agent", func(t *ftt.Test) {
			isRobot = false
			userAgent := "I am a human."

			UpdateServerMetrics(c, name, code, dur, 123, 321, userAgent)

			assert.That(t, serverDurationsMetric.Get(c, code, name, isRobot).Sum(), should.Equal(durMillis))
			assert.That(t, serverRequestBytesMetric.Get(c, code, name, isRobot).Sum(), should.Equal(123.0))
			assert.That(t, serverResponseBytesMetric.Get(c, code, name, isRobot).Sum(), should.Equal(321.0))
			assert.Loosely(t, serverResponseStatusMetric.Get(c, code, name, isRobot), should.Equal(1))
		})
	})
}
