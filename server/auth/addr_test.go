// Copyright 2015 The LUCI Authors.
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

package auth

import (
	"fmt"
	"net"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseRemoteIP(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test suites`, t, func(t *ftt.Test) {
		for _, tc := range []struct {
			v   string
			exp string
			err string
		}{
			{"", "0.0.0.0", ""},
			{"1.2.3.4", "1.2.3.4", ""},
			{"1.2.3.4:1337", "1.2.3.4", ""},
			{"1::1", "1::1", ""},
			{"[1::1]", "1::1", ""},
			{"[1::1]:1337", "1::1", ""},
			{"[abc:abc:abc:abc:abc:abc:abc:abc]:80", "abc:abc:abc:abc:abc:abc:abc:abc", ""},
			{"[1.2.3.4]:1337", "1.2.3.4", ""},
			{"[1::1:1337", "", "missing closing brace"},
			{"[1.2.3.4", "", "missing closing brace"},
			{"1.3.4:1337", "", "don't know how to parse"},
			{"1:3:5:9:1337", "", "don't know how to parse"},
		} {
			if tc.err == "" {
				t.Run(fmt.Sprintf(`Successfully parses %q into %q.`, tc.v, tc.exp), func(t *ftt.Test) {
					ip, err := parseRemoteIP(tc.v)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ip, should.Resemble(net.ParseIP(tc.exp)))
				})
			} else {
				t.Run(fmt.Sprintf(`Fails to parse %q with %q.`, tc.v, tc.err), func(t *ftt.Test) {
					_, err := parseRemoteIP(tc.v)
					assert.Loosely(t, err, should.ErrLike(tc.err))
				})
			}
		}
	})
}
