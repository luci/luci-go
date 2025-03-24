// Copyright 2025 The LUCI Authors.
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

package botapi

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/swarming/server/botstate"
)

func TestCheckBotHealthInfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		before string   // initial state dict
		dim    []string // quarantine message in the dimensions
		msg    string   // the expected quarantine message
		after  string   // the expected state dict after
	}{
		{"{}", nil, "", "{}"},
		{"{}", []string{"Boo"}, "Boo", "{\n  \"quarantined\": \"Boo\"\n}"},
		{"{\"quarantined\": true}", []string{"Boo"}, "Boo", "{\n  \"quarantined\": \"Boo\"\n}"},
		{"{\"quarantined\": \"Blah\"}", []string{"Boo"}, "Blah", "{\"quarantined\": \"Blah\"}"},
	}

	for _, cs := range cases {
		info, out, err := updateBotHealthInfo(botstate.Dict{JSON: []byte(cs.before)}, cs.dim, nil)
		assert.NoErr(t, err)
		assert.That(t, info.Quarantined, should.Equal(cs.msg))
		assert.That(t, string(out.JSON), should.Equal(cs.after))
	}
}
