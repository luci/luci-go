// Copyright 2024 The LUCI Authors.
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

package groupexporter

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStartOfWeek(t *testing.T) {
	ftt.Run("TestStartOfWeek", t, func(t *ftt.Test) {

		week := startOfWeek(time.Date(2024, 9, 14, 0, 0, 0, 0, time.UTC))
		expected := time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC)
		assert.That(t, week, should.Match(expected))

		week = startOfWeek(time.Date(2024, 9, 13, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = startOfWeek(time.Date(2024, 9, 12, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = startOfWeek(time.Date(2024, 9, 11, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = startOfWeek(time.Date(2024, 9, 10, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = startOfWeek(time.Date(2024, 9, 9, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

		week = startOfWeek(time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC))
		assert.That(t, week, should.Match(expected))

	})
}
