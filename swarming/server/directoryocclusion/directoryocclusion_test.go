// Copyright 2024 The LUCI Authors.
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

package directoryocclusion

import (
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDirectoryOcclusion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dirs map[string][]string // odd elements are owners, even elements are notes
		errs []string
	}{
		// good
		{
			name: "empty",
			dirs: map[string][]string{},
			errs: nil,
		},
		{
			name: "no_conflict_pathes_don't_overlap",
			dirs: map[string][]string{
				"a/b": {"owner1", "note"},
				"c/d": {"owner2", "note"},
				"c/e": {"owner3", "note"},
			},
			errs: nil,
		},
		{
			name: "no_conflict_pathes_all_same_user",
			dirs: map[string][]string{
				"a/b":     {"owner1", "note1", "owner1", "note2"},
				"a/b/c/d": {"owner1", "note1"},
			},
			errs: nil,
		},
		// conflicting
		{
			name: "conflict_different_owner_same_dir",
			dirs: map[string][]string{
				"a/b": {"owner1", "note1", "owner2", "note2"},
			},
			errs: []string{
				`"a/b": directory has conflicting owners: owner1[note1] and owner2[note2]`,
			},
		},
		{
			name: "conflict_different_owner_same_branch",
			dirs: map[string][]string{
				"a/b":     {"owner1", "note"},
				"a/b/c/d": {"owner2", "note"},
			},
			errs: []string{
				`owner2[note] uses "a/b/c/d", which conflicts with owner1[note] using "a/b"`,
			},
		},
		{
			name: "early_pruned_conflict",
			dirs: map[string][]string{
				"a/b": {"owner1", "note1", "owner2", "note2"},
				// This conflict will not be included in errs since the branch
				// has been pruned earlier.
				"a/b/c/d": {"owner3", "note"},
			},
			errs: []string{
				`"a/b": directory has conflicting owners: owner1[note1] and owner2[note2]`,
			},
		},
		{
			name: "multiple_conflicts",
			dirs: map[string][]string{
				"a/b":     {"owner1", "note", "owner1", "note1"},
				"a/b/c/d": {"owner2", "note"},
				"a/b/e":   {"owner3", "note"},
			},
			errs: []string{
				`owner2[note] uses "a/b/c/d", which conflicts with owner1[note, note1] using "a/b"`,
				`owner3[note] uses "a/b/e", which conflicts with owner1[note, note1] using "a/b"`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chk := NewChecker("")
			for dir, on := range tc.dirs {
				assert.That(t, len(on)%2, should.Equal(0))
				for i := 0; i < len(on); i += 2 {
					chk.Add(dir, on[i], on[i+1])
				}
			}
			var expected errors.MultiError
			for _, err := range tc.errs {
				expected = append(expected, errors.Reason(err).Err())
			}
			merr := chk.Conflicts()
			assert.That(t, len(merr), should.Equal(len(tc.errs)))
			for i, e := range merr {
				assert.Loosely(t, e, should.ErrLike(tc.errs[i]))
			}
		})
	}
}
