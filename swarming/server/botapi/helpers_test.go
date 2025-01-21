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
	"go.chromium.org/luci/swarming/server/model"
)

func TestBotHealthInfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state  any    // quarantine message in the state
		dims   string // quarantine message in the dimensions
		expect string // the expected message
	}{
		{},
		{"Boo", "", "Boo"},
		{"", "Boo", "Boo"},
		{true, "", model.GenericQuarantineMessage},
		{"", "True", model.GenericQuarantineMessage},
		{"Boo", "True", "Boo"},
		{true, "Boo", "Boo"},
	}

	for _, cs := range cases {
		state, err := botstate.Edit(botstate.Dict{}, func(d *botstate.EditableDict) error {
			if cs.state != nil {
				return d.Write(botstate.QuarantinedKey, cs.state)
			}
			return nil
		})
		assert.NoErr(t, err)

		dims := map[string][]string{}
		if cs.dims != "" {
			dims[botstate.QuarantinedKey] = []string{cs.dims}
		}

		info := botHealthInfo(&state, dims)
		assert.That(t, info.Quarantined, should.Equal(cs.expect))
	}
}

func TestUpdateQuarantineInState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		before  string
		message string
		after   string
	}{
		{"{}", "", "{}"},
		{"{}", "Boo", "{\n  \"quarantined\": \"Boo\"\n}"},
		{"{\"quarantined\": true}", "Boo", "{\n  \"quarantined\": \"Boo\"\n}"},
		{"{\"quarantined\": \"Blah\"}", "Boo", "{\"quarantined\": \"Blah\"}"},
	}

	for _, cs := range cases {
		out, err := updateQuarantineInState(botstate.Dict{JSON: []byte(cs.before)}, cs.message)
		assert.NoErr(t, err)
		assert.That(t, string(out.JSON), should.Equal(cs.after))
	}
}
