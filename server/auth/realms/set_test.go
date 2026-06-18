// Copyright 2026 The LUCI Authors.
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

package realms

import (
	"slices"
	"testing"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSet(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		entries     []string
		wantErr     string
		wantInside  []string
		wantOutside []string
	}{
		{
			name:        "empty",
			wantOutside: []string{"a:b"},
		},
		{
			name:       "universe",
			entries:    []string{"*", "project:ignored"},
			wantInside: []string{"a:b"},
		},
		{
			name:        "project_wildcard",
			entries:     []string{"a:*", "b:*"},
			wantInside:  []string{"a:123", "a:456", "b:789"},
			wantOutside: []string{"c:123", "ab:345"},
		},
		{
			name:        "full_realms",
			entries:     []string{"a:b", "c:d"},
			wantInside:  []string{"a:b", "c:d"},
			wantOutside: []string{"a:1"},
		},
		{
			name:        "mixed",
			entries:     []string{"a:1", "a:*", "b:2"},
			wantInside:  []string{"a:z", "a:1", "b:2"},
			wantOutside: []string{"b:z", "c:1"},
		},
		{
			name:        "exotic",
			entries:     []string{"@internal:*", "abc:@root"},
			wantInside:  []string{"@internal:abc", "abc:@root"},
			wantOutside: []string{"abc:other"},
		},
		{
			name:    "bad_project",
			entries: []string{"BAD:*"},
			wantErr: `entry #1: bad project name "BAD"`,
		},
		{
			name:    "bad_realm",
			entries: []string{"bad"},
			wantErr: `entry #1: bad global realm name "bad"`,
		},
	}

	for _, cs := range cases {
		for _, direction := range []string{"forward", "reverse"} {
			t.Run(cs.name, func(t *testing.T) {
				entries := slices.Clone(cs.entries)
				if direction == "reverse" {
					// Order matters since e.g. seeing "*" clears all other entries.
					slices.Reverse(entries)
				}
				set, err := NewSet(entries)
				if cs.wantErr != "" {
					assert.That(t, err, should.ErrLike(cs.wantErr))
				} else {
					assert.NoErr(t, err)
				}
				if cs.wantErr != "" {
					return
				}
				for _, val := range cs.wantInside {
					assert.That(t, set.Has(val), should.BeTrue, truth.Explain("%q", val))
				}
				for _, val := range cs.wantOutside {
					assert.That(t, set.Has(val), should.BeFalse, truth.Explain("%q", val))
				}
			})
		}
	}
}
