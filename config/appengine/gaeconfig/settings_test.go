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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/settings"
)

func TestSettingsURLToHostTranslation(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing settings configuration`, t, func(t *ftt.Test) {
		c := context.Background()

		memSettings := settings.MemoryStorage{}
		c = settings.Use(c, settings.New(&memSettings))

		t.Run(`Loaded with a URL value in ConfigServiceHost`, func(t *ftt.Test) {
			s := Settings{
				ConfigServiceHost: "https://example.com/foo/bar",
			}
			assert.Loosely(t, s.SetIfChanged(c), should.BeNil)

			t.Run(`Will load the setting as a host.`, func(t *ftt.Test) {
				s, err := FetchCachedSettings(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match(Settings{
					ConfigServiceHost: "example.com",
				}))
			})
		})

		t.Run(`Loaded with a host value in ConfigServiceHost`, func(t *ftt.Test) {
			s := Settings{
				ConfigServiceHost: "example.com",
			}
			assert.Loosely(t, s.SetIfChanged(c), should.BeNil)

			t.Run(`Will load the setting as a host.`, func(t *ftt.Test) {
				s, err := FetchCachedSettings(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match(Settings{
					ConfigServiceHost: "example.com",
				}))
			})
		})
	})
}
