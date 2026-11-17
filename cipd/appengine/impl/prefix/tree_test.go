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

package prefix

import (
	"fmt"
	"iter"
	"slices"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type kv[V any] struct {
	k string
	v V
}

func seqOf[V any](items ...kv[V]) iter.Seq2[string, V] {
	return func(yield func(string, V) bool) {
		for _, item := range items {
			if !yield(item.k, item.v) {
				return
			}
		}
	}
}

func TestNormPathAndNextSegment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in       string
		wantNorm string
		wantSegs []string
	}{
		{"", "", nil},
		{"/", "", nil},
		{"///", "", nil},
		{"a", "a/", []string{"a"}},
		{"/a", "a/", []string{"a"}},
		{"a/", "a/", []string{"a"}},
		{"/a/", "a/", []string{"a"}},
		{"a/b/c", "a/b/c/", []string{"a", "b", "c"}},
		{"/a/b/c/", "a/b/c/", []string{"a", "b", "c"}},
		{"a//b///c", "a/b/c/", []string{"a", "b", "c"}},
		{"//a//b/c//", "a/b/c/", []string{"a", "b", "c"}},
	}

	for _, tc := range cases {
		assert.That(t, normPath(tc.in), should.Equal(tc.wantNorm))
		cur := tc.in
		nextSegments := slices.Collect(func(yield func(string) bool) {
			var seg string
			var ok bool
			for {
				seg, cur, ok = nextSegment(cur)
				if !ok || !yield(seg) {
					return
				}
			}
		})
		assert.That(t, nextSegments, should.Match(tc.wantSegs))
	}
}

func TestTreeGet(t *testing.T) {
	t.Parallel()

	t.Run("nil and zero-value Tree return zero V", func(t *testing.T) {
		var nilTree *Tree[string]
		var zeroTree Tree[string]
		assert.That(t, nilTree.Get("a/b"), should.Equal(""))
		assert.That(t, zeroTree.Get("a/b"), should.Equal(""))
	})

	t.Run("empty values uses fallback root", func(t *testing.T) {
		tr := New("root", 0, seqOf[string](), nil)
		assert.That(t, tr.Get(""), should.Equal("root"))
		assert.That(t, tr.Get("/"), should.Equal("root"))
		assert.That(t, tr.Get("a/b/c"), should.Equal("root"))
	})

	t.Run("explicit root overrides fallback root", func(t *testing.T) {
		tr := New("fallback", 2, seqOf(
			kv[string]{"/", "explicit"},
			kv[string]{"a/b", "ab"},
		), nil)
		assert.That(t, tr.Get(""), should.Equal("explicit"))
		assert.That(t, tr.Get("/"), should.Equal("explicit"))
		assert.That(t, tr.Get("a"), should.Equal("explicit"))
		assert.That(t, tr.Get("a/b"), should.Equal("ab"))
		assert.That(t, tr.Get("a/b/c"), should.Equal("ab"))
	})

	t.Run("duplicate normalized paths keep last value", func(t *testing.T) {
		tr := New("fallback", 4, seqOf(
			kv[string]{"a/b", "first"},
			kv[string]{"/a/b/", "second"},
			kv[string]{"", "r1"},
			kv[string]{"/", "r2"},
		), nil)
		assert.That(t, tr.Get(""), should.Equal("r2"))
		assert.That(t, tr.Get("a/b"), should.Equal("second"))
	})

	t.Run("compressed edges and nested intermediate nodes", func(t *testing.T) {
		// Structure:
		//   "" ("root")
		//   └── "a" (intermediate)
		//       ├── "b/c" (intermediate)
		//       │   ├── "d/x" ("abcdx")
		//       │   └── "e" ("abce")
		//       └── "f" ("af")
		tr := New("root", 3, seqOf(
			kv[string]{"a/b/c/e", "abce"},
			kv[string]{"a/f", "af"},
			kv[string]{"a/b/c/d/x", "abcdx"},
		), nil)

		// Exact node count: root + "a" + "b/c" + "f" + "d/x" + "e" = 6.
		assert.That(t, len(tr.nodes), should.Equal(6))
		assert.That(t, cap(tr.nodes), should.Equal(6))

		for _, q := range []string{"", "/", "z", "a", "a/b", "a/b/c", "a/b/z", "a/b/c/d", "a/b/c/d/y"} {
			assert.That(t, tr.Get(q), should.Equal("root"))
		}
		assert.That(t, tr.Get("a/f"), should.Equal("af"))
		assert.That(t, tr.Get("a/f/sub"), should.Equal("af"))
		assert.That(t, tr.Get("a/b/c/e"), should.Equal("abce"))
		assert.That(t, tr.Get("a/b/c/e/sub"), should.Equal("abce"))
		assert.That(t, tr.Get("a/b/c/d/x"), should.Equal("abcdx"))
		assert.That(t, tr.Get("//a//b///c/d/x/deep//"), should.Equal("abcdx"))
	})

	t.Run("segment characters around slash and shared segment prefixes", func(t *testing.T) {
		tr := New("root", 8, seqOf(
			kv[string]{"a", "a"},
			kv[string]{"a/b", "ab"},
			kv[string]{"a/b/c", "abc"},
			kv[string]{"a/b-c", "ab-dash"},
			kv[string]{"a/b.c", "ab-dot"},
			kv[string]{"a/b_c", "ab-under"},
			kv[string]{"pkg/linux-amd64", "amd64"},
			kv[string]{"pkg/linux-arm64", "arm64"},
		), nil)

		// Verify pkg/linux-amd64 and pkg/linux-arm64 split at segment "pkg",
		// not mid-segment at "linux-a".
		assert.That(t, len(tr.nodes), should.Equal(10))
		assert.That(t, cap(tr.nodes), should.Equal(10))

		assert.That(t, tr.Get("a"), should.Equal("a"))
		assert.That(t, tr.Get("a/b"), should.Equal("ab"))
		assert.That(t, tr.Get("a/b/c"), should.Equal("abc"))
		assert.That(t, tr.Get("a/b/d"), should.Equal("ab"))
		assert.That(t, tr.Get("a/b-a"), should.Equal("a"))
		assert.That(t, tr.Get("a/b-c"), should.Equal("ab-dash"))
		assert.That(t, tr.Get("a/b-c/d"), should.Equal("ab-dash"))
		assert.That(t, tr.Get("a/b-c-2"), should.Equal("a"))
		assert.That(t, tr.Get("a/b-z"), should.Equal("a"))
		assert.That(t, tr.Get("a/b.c"), should.Equal("ab-dot"))
		assert.That(t, tr.Get("a/b.z"), should.Equal("a"))
		assert.That(t, tr.Get("a/b0"), should.Equal("a"))
		assert.That(t, tr.Get("a/b_a"), should.Equal("a"))
		assert.That(t, tr.Get("a/b_c"), should.Equal("ab-under"))
		assert.That(t, tr.Get("a/b_z"), should.Equal("a"))

		assert.That(t, tr.Get("pkg"), should.Equal("root"))
		assert.That(t, tr.Get("pkg/linux-a"), should.Equal("root"))
		assert.That(t, tr.Get("pkg/linux-amd64"), should.Equal("amd64"))
		assert.That(t, tr.Get("pkg/linux-arm64"), should.Equal("arm64"))
	})
}

func TestTreeGetNoAllocs(t *testing.T) {
	// Not parallel: testing.AllocsPerRun panics if any parallel test is active.
	tr := New("root", 2, seqOf(
		kv[string]{"a", "a"},
		kv[string]{"a/b/c/d", "abcd"},
	), nil)
	allocs := testing.AllocsPerRun(100, func() {
		_ = tr.Get("a/b/c/d/e/f")
	})
	assert.That(t, allocs, should.Equal(0.0))
}

func TestTreeMerge(t *testing.T) {
	t.Parallel()

	var mergeCalls []string
	merge := func(parent, child string) string {
		merged := parent + "->" + child
		mergeCalls = append(mergeCalls, merged)
		return merged
	}

	// "a/b" is an intermediate between "a/b/c" and "a/b/d"; both children
	// must inherit directly from "a" without calling merge on "a/b".
	tr := New("R", 4, seqOf(
		kv[string]{"a/b/d", "D"},
		kv[string]{"a", "A"},
		kv[string]{"a/b/c/deep", "DEEP"},
		kv[string]{"a/b/c", "C"},
	), merge)

	assert.That(t, tr.Get(""), should.Equal("R"))
	assert.That(t, tr.Get("a"), should.Equal("R->A"))
	assert.That(t, tr.Get("a/b"), should.Equal("R->A"))
	assert.That(t, tr.Get("a/b/c"), should.Equal("R->A->C"))
	assert.That(t, tr.Get("a/b/c/deep"), should.Equal("R->A->C->DEEP"))
	assert.That(t, tr.Get("a/b/d"), should.Equal("R->A->D"))

	slices.Sort(mergeCalls)
	assert.That(t, mergeCalls, should.Match([]string{
		"R->A",
		"R->A->C",
		"R->A->C->DEEP",
		"R->A->D",
	}))

	t.Run("explicit root merges with default root", func(t *testing.T) {
		trRoot := New("DEFAULT", 2, seqOf(
			kv[string]{"/", "EXPLICIT"},
			kv[string]{"a", "A"},
		), func(parent, child string) string {
			return parent + "->" + child
		})
		assert.That(t, trRoot.Get(""), should.Equal("DEFAULT->EXPLICIT"))
		assert.That(t, trRoot.Get("a"), should.Equal("DEFAULT->EXPLICIT->A"))
	})

	t.Run("duplicate explicit roots merge only winning root in top-down order", func(t *testing.T) {
		var calls []string
		trDup := New("DEFAULT", 4, seqOf(
			kv[string]{"", "R1"},
			kv[string]{"a", "A1"},
			kv[string]{"/", "R2"},
			kv[string]{"a/", "A2"},
		), func(parent, child string) string {
			merged := parent + "->" + child
			calls = append(calls, merged)
			return merged
		})
		assert.That(t, trDup.Get(""), should.Equal("DEFAULT->R2"))
		assert.That(t, trDup.Get("a"), should.Equal("DEFAULT->R2->A2"))
		assert.That(t, calls, should.Match([]string{
			"DEFAULT->R2",
			"DEFAULT->R2->A2",
		}))
	})
}

func TestNodeShapes(t *testing.T) {
	t.Parallel()

	t.Run("256 chained nodes", func(t *testing.T) {
		items := make([]kv[int], 255)
		cur := ""
		for i := range 255 {
			if i == 0 {
				cur = "s"
			} else {
				cur += "/s"
			}
			items[i] = kv[int]{cur, i + 1}
		}
		tr := New(0, -1, seqOf(items...), nil)
		assert.That(t, len(tr.nodes), should.Equal(256))
		assert.That(t, cap(tr.nodes), should.Equal(256))
		assert.That(t, tr.Get(""), should.Equal(0))
		assert.That(t, tr.Get(items[0].k), should.Equal(1))
		assert.That(t, tr.Get(items[254].k), should.Equal(255))
		assert.That(t, tr.Get(items[254].k+"/extra"), should.Equal(255))
	})

	t.Run("256 wide nodes", func(t *testing.T) {
		items := make([]kv[int], 255)
		for i := range 255 {
			items[i] = kv[int]{fmt.Sprintf("c%03d", i), i + 1}
		}
		tr := New(0, 255, seqOf(items...), nil)
		assert.That(t, len(tr.nodes), should.Equal(256))
		assert.That(t, cap(tr.nodes), should.Equal(256))
		for i, it := range items {
			assert.That(t, tr.Get(it.k), should.Equal(i+1))
		}
	})

	t.Run("many intermediate nodes", func(t *testing.T) {
		// 1 root + 90 intermediate nodes ("gXX") + 180 leaves = 271 nodes.
		var items []kv[int]
		for g := range 90 {
			items = append(items,
				kv[int]{fmt.Sprintf("g%02d/a", g), g*2 + 1},
				kv[int]{fmt.Sprintf("g%02d/b", g), g*2 + 2},
			)
		}
		tr := New(0, len(items), seqOf(items...), nil)
		assert.That(t, len(tr.nodes), should.Equal(271))
		assert.That(t, cap(tr.nodes), should.Equal(271))

		for g := range 90 {
			assert.That(t, tr.Get(fmt.Sprintf("g%02d", g)), should.Equal(0))
			assert.That(t, tr.Get(fmt.Sprintf("g%02d/a", g)), should.Equal(g*2+1))
			assert.That(t, tr.Get(fmt.Sprintf("g%02d/b", g)), should.Equal(g*2+2))
		}
	})
}
