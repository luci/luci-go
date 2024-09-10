// Copyright 2020 The LUCI Authors.
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

package prjcfg

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

func TestGerritHost(t *testing.T) {
	t.Parallel()

	ftt.Run("GerritHost", t, func(t *ftt.Test) {
		assert.Loosely(t, GerritHost(&cfgpb.ConfigGroup_Gerrit{Url: "https://o.k"}), should.Equal("o.k"))
		assert.Loosely(t, GerritHost(&cfgpb.ConfigGroup_Gerrit{Url: "https://strip.sla.shes/"}), should.Equal("strip.sla.shes"))

		assert.Loosely(t, func() { GerritHost(&cfgpb.ConfigGroup_Gerrit{}) }, should.Panic)
		assert.Loosely(t, func() { GerritHost(&cfgpb.ConfigGroup_Gerrit{Url: "no.scheme"}) }, should.Panic)
	})
}
