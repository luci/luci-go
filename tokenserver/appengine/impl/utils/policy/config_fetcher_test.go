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

package policy

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
)

func TestConfigFetcher(t *testing.T) {
	t.Parallel()

	ftt.Run("Fetches bunch of configs", t, func(t *ftt.Test) {
		c := gaetesting.TestingContext()
		c = prepareServiceConfig(c, map[string]string{
			"abc.cfg": "seconds: 12345",
			"def.cfg": "seconds: 67890",
		})

		f := luciConfigFetcher{}
		ts := timestamppb.Timestamp{} // using timestamp as guinea pig proto message

		assert.Loosely(t, f.FetchTextProto(c, "missing", &ts), should.Equal(config.ErrNoConfig))
		assert.Loosely(t, f.Revision(), should.BeEmpty)

		assert.Loosely(t, f.FetchTextProto(c, "abc.cfg", &ts), should.BeNil)
		assert.Loosely(t, ts.Seconds, should.Equal(12345))
		assert.Loosely(t, f.FetchTextProto(c, "def.cfg", &ts), should.BeNil)
		assert.Loosely(t, ts.Seconds, should.Equal(67890))

		assert.Loosely(t, f.Revision(), should.Equal("20a03c9df37ce4413c01b580a8ae48a0aafce038"))
	})

	ftt.Run("Revision changes midway", t, func(t *ftt.Test) {
		base := gaetesting.TestingContext()

		f := luciConfigFetcher{}
		ts := timestamppb.Timestamp{}

		c1 := prepareServiceConfig(base, map[string]string{
			"abc.cfg": "seconds: 12345",
		})
		assert.Loosely(t, f.FetchTextProto(c1, "abc.cfg", &ts), should.BeNil)
		assert.Loosely(t, ts.Seconds, should.Equal(12345))

		c2 := prepareServiceConfig(base, map[string]string{
			"def.cfg": "seconds: 12345",
		})
		assert.Loosely(t, f.FetchTextProto(c2, "def.cfg", &ts), should.ErrLike(
			`expected config "def.cfg" to be at rev f4b3d57ff7afec86b3850f40713008ff4a494bda, `+
				`but got 32c954f97d0dae611fff581a39bca4a56686285b`))
	})
}

func prepareServiceConfig(c context.Context, configs map[string]string) context.Context {
	return cfgclient.Use(c, memory.New(map[config.Set]memory.Files{
		"services/${appid}": configs,
	}))
}
