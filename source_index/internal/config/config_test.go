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

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Config", t, func() {
		var cfg Config = &config{TestCfg}

		Convey("HasHost", func() {
			// Match one host.
			So(cfg.HasHost("chromium.googlesource.com"), ShouldBeTrue)

			// Match another host.
			So(cfg.HasHost("webrtc.googlesource.com"), ShouldBeTrue)

			// Do not match any host.
			So(cfg.HasHost("another-host.googlesource.com"), ShouldBeFalse)
		})

		Convey("ShouldIndexRef", func() {
			// Match regex.
			So(cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/branch-heads/release-101"), ShouldBeTrue)

			// Match another regex.
			So(cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/heads/main"), ShouldBeTrue)

			// Don't match any regex.
			So(cfg.ShouldIndexRef("chromium.googlesource.com", "chromium/src", "refs/heads/another-branch"), ShouldBeFalse)

			// Match regex in another repo.
			So(cfg.ShouldIndexRef("chromium.googlesource.com", "chromiumos/manifest", "refs/heads/staging-snapshot"), ShouldBeTrue)

			// Do not match any repo.
			So(cfg.ShouldIndexRef("chromium.googlesource.com", "another-repo", "refs/heads/main"), ShouldBeFalse)

			// Match regex in another host.
			So(cfg.ShouldIndexRef("webrtc.googlesource.com", "src", "refs/heads/main"), ShouldBeTrue)

			// Do not match any host.
			So(cfg.ShouldIndexRef("another-host.googlesource.com", "src", "refs/heads/main"), ShouldBeFalse)
		})
	})
}
