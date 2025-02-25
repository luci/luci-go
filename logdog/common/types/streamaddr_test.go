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

package types

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStreamAddr(t *testing.T) {
	t.Parallel()

	var successes = []struct {
		s   string
		exp StreamAddr
	}{
		{"logdog://host/project/a/+/b", StreamAddr{"host", "project", "a/+/b"}},
		{"logdog://host.example.com/project/foo/bar/+/baz", StreamAddr{"host.example.com", "project", "foo/bar/+/baz"}},
	}

	var failures = []struct {
		s   string
		err string
	}{
		{"://project/prefix/+/name", "failed to parse URL"},
		{"http://example.com/foo/bar/+/baz", "is not logdog"},
		{"logdog://example.com/foo", "URL path does not include both project and path components"},
		{"logdog://example.com/foo@d/bar", "invalid project name"},
		{"logdog://example.com/foo/bar", "invalid stream path"},
		{"logdog://example.com/foo/bar/+/ba!", "invalid stream path"},
	}

	ftt.Run(`Testing StreamAddr`, t, func(t *ftt.Test) {

		for _, tc := range successes {
			t.Run(fmt.Sprintf(`Success: %q`, tc.s), func(t *ftt.Test) {
				addr, err := ParseURL(tc.s)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, addr, should.Match(&tc.exp))

				u, err := url.Parse(tc.s)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, addr.URL(), should.Match(u))
			})
		}

		for _, tc := range failures {
			t.Run(fmt.Sprintf(`Failure: %q fails like: %q`, tc.s, tc.err), func(t *ftt.Test) {
				_, err := ParseURL(tc.s)
				assert.Loosely(t, err, should.ErrLike(tc.err))
			})
		}
	})

	ftt.Run(`StreamAddr is a flag.Value`, t, func(t *ftt.Test) {
		fs := flag.NewFlagSet("testing", flag.ContinueOnError)
		a := &StreamAddr{}

		fs.Var(a, "addr", "its totally an address of a thing")

		t.Run(`good`, func(t *ftt.Test) {
			assert.Loosely(t, fs.Parse([]string{"-addr", "logdog://host/project/a/+/b"}), should.BeNil)
			assert.Loosely(t, a, should.Match(&StreamAddr{
				"host",
				"project",
				"a/+/b",
			}))
		})

		t.Run(`bad`, func(t *ftt.Test) {
			assert.Loosely(t, fs.Parse([]string{"-addr", "://host/project/a/+/b"}), should.ErrLike(
				"failed to parse URL"))
		})
	})

	ftt.Run(`StreamAddr as a json value`, t, func(t *ftt.Test) {
		a := &StreamAddr{}

		t.Run(`good`, func(t *ftt.Test) {
			t.Run(`zero`, func(t *ftt.Test) {
				data, err := json.Marshal(a)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(data), should.Match(`{}`))
				assert.Loosely(t, json.Unmarshal(data, a), should.BeNil)
				assert.Loosely(t, a, should.Match(&StreamAddr{}))
			})

			t.Run(`full`, func(t *ftt.Test) {
				a.Host = "host"
				a.Project = "project"
				a.Path = "a/+/b"
				data, err := json.Marshal(a)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(data), should.Match(`{"host":"host","project":"project","path":"a/+/b"}`))

				a2 := &StreamAddr{}
				assert.Loosely(t, json.Unmarshal(data, a2), should.BeNil)
				assert.Loosely(t, a2, should.Match(a))
			})
		})

		t.Run(`bad`, func(t *ftt.Test) {
			assert.Loosely(t, json.Unmarshal([]byte(`{"host":"host","project":"project","path":"fake"}`), a), should.ErrLike(
				"must contain at least one character")) // from bad Path
		})
	})
}
