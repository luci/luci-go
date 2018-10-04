// Copyright 2017 The LUCI Authors.
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

package gaeconfig

import (
	"context"
	"testing"

	"go.chromium.org/luci/server/settings"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSettingsURLToHostTranslation(t *testing.T) {
	t.Parallel()

	Convey(`A testing settings configuration`, t, func() {
		c := context.Background()

		memSettings := settings.MemoryStorage{}
		c = settings.Use(c, settings.New(&memSettings))

		Convey(`Loaded with a URL value in ConfigServiceHost`, func() {
			s := Settings{
				ConfigServiceHost: "https://example.com/foo/bar",
			}
			So(s.SetIfChanged(c, "test harness", "initial settings"), ShouldBeNil)

			Convey(`Will load the setting as a host.`, func() {
				s, err := FetchCachedSettings(c)
				So(err, ShouldBeNil)
				So(s, ShouldResemble, Settings{
					ConfigServiceHost: "example.com",
				})
			})
		})

		Convey(`Loaded with a host value in ConfigServiceHost`, func() {
			s := Settings{
				ConfigServiceHost: "example.com",
			}
			So(s.SetIfChanged(c, "test harness", "initial settings"), ShouldBeNil)

			Convey(`Will load the setting as a host.`, func() {
				s, err := FetchCachedSettings(c)
				So(err, ShouldBeNil)
				So(s, ShouldResemble, Settings{
					ConfigServiceHost: "example.com",
				})
			})
		})
	})
}
