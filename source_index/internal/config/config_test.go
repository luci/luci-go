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

package config

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Config", t, func(t *ftt.Test) {
		var cfg Config = &config{TestCfg}

		t.Run("HasHost", func(t *ftt.Test) {
			// Match one host.
			assert.That(t, cfg.HasHost("chromium.googlesource.com"), should.BeTrue)

			// Match another host.
			assert.That(t, cfg.HasHost("webrtc.googlesource.com"), should.BeTrue)

			// Do not match any host.
			assert.That(t, cfg.HasHost("another-host.googlesource.com"), should.BeFalse)
		})

		t.Run("ShouldIndexRef", func(t *ftt.Test) {
			// Match regex.
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/branch-heads/release-101"), should.BeTrue)

			// Match regex but not an indexable ref.
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/arbitrary/release-101"), should.BeFalse)

			// Match another regex.
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/heads/main"), should.BeTrue)

			// Do not match regex after the regex is wrapped in "^...$".
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/heads/mainsuffix"), should.BeFalse)

			// Don't match any regex.
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/heads/another-branch"), should.BeFalse)

			// Match regex in another repo.
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "chromiumos/manifest", "refs/heads/staging-snapshot"), should.BeTrue)

			// Do not match any repo.
			assert.That(t, cfg.ShouldIndexRef("chromium.googlesource.com", "another-repo", "refs/heads/main"), should.BeFalse)

			// Match regex in another host.
			assert.That(t, cfg.ShouldIndexRef("webrtc.googlesource.com", "src", "refs/heads/main"), should.BeTrue)

			// Do not match any host.
			assert.That(t, cfg.ShouldIndexRef("another-host.googlesource.com", "src", "refs/heads/main"), should.BeFalse)
		})
	})
}
