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

package logdog

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetLogHostValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("GetLog Host and Scheme Validation", t, func(t *ftt.Test) {
		ctx := context.Background()
		client := &LogdogClient{}

		t.Run("Rejects non-https scheme", func(t *ftt.Test) {
			_, err := client.GetLog(ctx, "http://logs.chromium.org/logs/chrome/test")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid logdog url scheme"))
		})

		t.Run("Rejects untrusted external host", func(t *ftt.Test) {
			_, err := client.GetLog(ctx, "https://attacker.example/logs/chrome/leak")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("untrusted logdog host"))
		})

		t.Run("Rejects arbitrary appspot host", func(t *ftt.Test) {
			_, err := client.GetLog(ctx, "https://attacker-app.appspot.com/logs/chrome/leak")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("untrusted logdog host"))
		})

		t.Run("Valid logdog url passes scheme and host validation", func(t *ftt.Test) {
			_, err := client.GetLog(ctx, "https://logs.chromium.org/logs/chrome/test")
			if err != nil {
				assert.Loosely(t, err.Error(), should.NotContainSubstring("invalid logdog url scheme"))
				assert.Loosely(t, err.Error(), should.NotContainSubstring("untrusted logdog host"))
			}
		})
	})
}
