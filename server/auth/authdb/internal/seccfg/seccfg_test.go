// Copyright 2019 The LUCI Authors.
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

package seccfg

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestSecurityConfig(t *testing.T) {
	ftt.Run("Empty", t, func(t *ftt.Test) {
		cfg, err := Parse(nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfg.IsInternalService("something.example.com"), should.BeFalse)
	})

	ftt.Run("IsInternalService works", t, func(t *ftt.Test) {
		blob, _ := proto.Marshal(&protocol.SecurityConfig{
			InternalServiceRegexp: []string{
				`(.*-dot-)?i1\.example\.com`,
				`(.*-dot-)?i2\.example\.com`,
			},
		})
		cfg, err := Parse(blob)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, cfg.IsInternalService("i1.example.com"), should.BeTrue)
		assert.Loosely(t, cfg.IsInternalService("i2.example.com"), should.BeTrue)
		assert.Loosely(t, cfg.IsInternalService("abc-dot-i1.example.com"), should.BeTrue)
		assert.Loosely(t, cfg.IsInternalService("external.example.com"), should.BeFalse)
		assert.Loosely(t, cfg.IsInternalService("something-i1.example.com"), should.BeFalse)
		assert.Loosely(t, cfg.IsInternalService("i1.example.com-something"), should.BeFalse)
	})
}
