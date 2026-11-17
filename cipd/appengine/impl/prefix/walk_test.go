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
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWalkEmptyAndLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("no tracks, nil trees, or zero-value trees yield nothing", func(t *testing.T) {
		for range Walk("") {
			t.Fatal("expected 0 iterations for 0 tracks")
		}

		var zeroTrack Track[int]
		var zeroTree Tree[string]
		nilTrack := NewTrack[string](nil)
		for range Walk("a/b", &zeroTrack, nilTrack, zeroTree.Track()) {
			t.Fatal("expected 0 iterations when all tracks are unbound or empty")
		}
	})

	t.Run("retained strings and early break", func(t *testing.T) {
		tr := New("root", 3, seqOf(
			kv[string]{"a/b", "ab"},
			kv[string]{"a/c", "ac"},
			kv[string]{"d", "d"},
		), nil)

		var earlyStop []string
		for p := range Walk("", tr.Track()) {
			earlyStop = append(earlyStop, p)
			if len(earlyStop) == 2 {
				break
			}
		}
		assert.That(t, earlyStop, should.Match([]string{"", "a/b"}))

		var retained []string
		for p := range Walk("", NewTrack(tr)) {
			retained = append(retained, p)
		}
		assert.That(t, retained, should.Match([]string{
			"",
			"a/b",
			"a/c",
			"d",
		}))
	})

	t.Run("single-use track panics on duplicate, nested, or second Walk", func(t *testing.T) {
		tr := New("root", 1, seqOf(kv[string]{"a", "a"}), nil)

		// Duplicate argument in same Walk.
		dup := NewTrack(tr)
		assert.That(t, func() {
			for range Walk("", dup, dup) {
			}
		}, should.Panic)

		// Nested Walk and post-Walk reuse.
		track := NewTrack(tr)
		var got []string
		for p := range Walk("", track) {
			got = append(got, p)
			assert.That(t, func() {
				for range Walk("", track) {
				}
			}, should.Panic)
		}
		assert.That(t, got, should.Match([]string{"", "a"}))

		// Second Walk after completion.
		assert.That(t, func() {
			for range Walk("", track) {
			}
		}, should.Panic)

		// Second Walk after early break.
		broken := NewTrack(tr)
		for range Walk("", broken) {
			break
		}
		assert.That(t, func() {
			for range Walk("", broken) {
			}
		}, should.Panic)
	})

	t.Run("SetTree before Walk sets initial tree", func(t *testing.T) {
		var track Track[string]
		track.SetTree(New("R", 1, seqOf(kv[string]{"x", "X"}), nil))

		var got []string
		for p := range Walk("", &track) {
			got = append(got, p+":"+track.Value())
		}
		assert.That(t, got, should.Match([]string{":R", "x:X"}))
	})

	t.Run("Value and Prune panic outside active Walk", func(t *testing.T) {
		track := NewTrack(New("R", 1, seqOf(kv[string]{"x", "X"}), nil))
		assert.That(t, func() { _ = track.Value() }, should.Panic)
		assert.That(t, func() { track.Prune() }, should.Panic)

		for range Walk("", track) {
		}

		assert.That(t, func() { _ = track.Value() }, should.Panic)
		assert.That(t, func() { track.Prune() }, should.Panic)
	})
}

func TestWalkOrderAndStartPaths(t *testing.T) {
	t.Parallel()

	t.Run("segment-DFS order with punctuation and unconfigured LCAs", func(t *testing.T) {
		// Includes characters sorting before and after '/' in ASCII ('-', '.', '_')
		// plus unconfigured LCA split nodes ("pkg", "deep", "deep/lca").
		tr := New("root", 9, seqOf(
			kv[string]{"pkg/linux-arm64", "arm64"},
			kv[string]{"pkg/linux-amd64", "amd64"},
			kv[string]{"a/b_c", "ab-under"},
			kv[string]{"a/b.c", "ab-dot"},
			kv[string]{"a/b-c", "ab-dash"},
			kv[string]{"a/b/d", "abd"},
			kv[string]{"a/b", "ab"},
			kv[string]{"a", "a"},
			kv[string]{"deep/lca/one/leaf", "one"},
			kv[string]{"deep/lca/two", "two"},
		), nil)

		track := NewTrack(tr)
		type visit struct {
			Path string
			Val  string
		}
		var visits []visit
		for p := range Walk("", track) {
			visits = append(visits, visit{p, track.Value()})
		}

		// With trailing-slash byte order ('-' < '.' < '/' < '_'), "a/b-c" and
		// "a/b.c" precede "a/b" -> "a/b/d" -> "a/b_c", while every subtree stays
		// contiguous and unconfigured LCAs ("pkg", "deep", "deep/lca") are skipped.
		assert.That(t, visits, should.Match([]visit{
			{"", "root"},
			{"a", "a"},
			{"a/b-c", "ab-dash"},
			{"a/b.c", "ab-dot"},
			{"a/b", "ab"},
			{"a/b/d", "abd"},
			{"a/b_c", "ab-under"},
			{"deep/lca/one/leaf", "one"},
			{"deep/lca/two", "two"},
			{"pkg/linux-amd64", "amd64"},
			{"pkg/linux-arm64", "arm64"},
		}))
	})

	t.Run("startPath at exact node, LCA, mid-edge, and diverged path", func(t *testing.T) {
		tr := New("root", 3, seqOf(
			kv[string]{"a", "A"},
			kv[string]{"a/b/c/d/x", "X"},
			kv[string]{"a/b/c/e", "E"},
		), nil)

		collect := func(start string) []string {
			track := NewTrack(tr)
			var out []string
			for p := range Walk(start, track) {
				out = append(out, fmt.Sprintf("%s=%s", p, track.Value()))
			}
			return out
		}

		// Mid-edge start on compressed edge "b/c" under "a".
		assert.That(t, collect("a/b"), should.Match([]string{
			"a/b=A",
			"a/b/c/d/x=X",
			"a/b/c/e=E",
		}))

		// Unconfigured LCA start at "a/b/c".
		assert.That(t, collect("/a/b/c/"), should.Match([]string{
			"a/b/c=A",
			"a/b/c/d/x=X",
			"a/b/c/e=E",
		}))

		// Mid-edge start on leaf edge "d/x" under "a/b/c".
		assert.That(t, collect("a/b/c/d"), should.Match([]string{
			"a/b/c/d=A",
			"a/b/c/d/x=X",
		}))

		// Diverged mid-edge start path inherits nearest ancestor ("A") and stops.
		assert.That(t, collect("a/b/other/sub"), should.Match([]string{
			"a/b/other/sub=A",
		}))

		// Start paths missing at root's children, at "a"'s children, or past a leaf.
		assert.That(t, collect("z"), should.Match([]string{
			"z=root",
		}))
		assert.That(t, collect("a/other/sub"), should.Match([]string{
			"a/other/sub=A",
		}))
		assert.That(t, collect("a/b/c/e/deep"), should.Match([]string{
			"a/b/c/e/deep=E",
		}))
	})
}

func TestWalkMultiTracksAndDynamicControl(t *testing.T) {
	t.Parallel()

	t.Run("heterogeneous tracks with overlapping compressed edges", func(t *testing.T) {
		// t1 has an explicit node at "a/b" and a sibling "a/z" (making "a" an LCA
		// node in t1), while t2 has a single compressed edge "a/m/c/d" & "a/m/c/e"
		// crossing right over "a" (so t2 sits mid-edge at "a" while t1 visits
		// "a/b" before "a/m" and "a/z" after "a/m").
		t1 := New("r1", 4, seqOf(
			kv[string]{"a/b", "ab1"},
			kv[string]{"a/m", "am1"},
			kv[string]{"a/z", "az1"},
			kv[string]{"m", "m1"},
		), nil)
		t2 := New(0, 2, seqOf(
			kv[int]{"a/m/c/d", 42},
			kv[int]{"a/m/c/e", 99},
		), nil)

		tr1 := NewTrack(t1)
		tr2 := NewTrack(t2)

		type row struct {
			Path string
			V1   string
			V2   int
		}
		var rows []row
		for p := range Walk("", tr1, tr2) {
			rows = append(rows, row{
				Path: p,
				V1:   tr1.Value(),
				V2:   tr2.Value(),
			})
		}
		assert.That(t, rows, should.Match([]row{
			{"", "r1", 0},
			{"a/b", "ab1", 0},
			{"a/m", "am1", 0},
			{"a/m/c/d", "am1", 42},
			{"a/m/c/e", "am1", 99},
			{"a/z", "az1", 0},
			{"m", "m1", 0},
		}))

		// Verify single-pass nextPrefix is order-independent across all 6
		// permutations of 3 tracks with staggered compressed edges.
		ta := New("ra", 2, seqOf(kv[string]{"b/x", "bx"}, kv[string]{"a/d", "ad"}), nil)
		tb := New("rb", 1, seqOf(kv[string]{"a/b/c", "abc"}), nil)
		tc := New("rc", 1, seqOf(kv[string]{"a/b/e", "abe"}), nil)
		perms := [][3]int{
			{0, 1, 2}, {0, 2, 1},
			{1, 0, 2}, {1, 2, 0},
			{2, 0, 1}, {2, 1, 0},
		}
		for _, perm := range perms {
			trA, trB, trC := NewTrack(ta), NewTrack(tb), NewTrack(tc)
			args := [3]GenericTrack{trA, trB, trC}
			var got []string
			for p := range Walk("", args[perm[0]], args[perm[1]], args[perm[2]]) {
				got = append(got, fmt.Sprintf("%s:%s,%s,%s", p, trA.Value(), trB.Value(), trC.Value()))
			}
			assert.That(t, got, should.Match([]string{
				":ra,rb,rc",
				"a/b/c:ra,abc,rc",
				"a/b/e:ra,rb,abe",
				"a/d:ad,rb,rc",
				"b/x:bx,rb,rc",
			}))
		}
	})

	t.Run("CIPD 3-tree pattern with subtree SetTree and Prune", func(t *testing.T) {
		type svcCfg struct {
			project string
			tag     string
		}

		// Service config:
		//   ""            -> unowned ("")
		//   "a"           -> owned by "p1"
		//   "a/b/sub"     -> delegated to "p2"
		//   "a/b/unowned" -> explicitly unowned ("")
		//   "a/sib"       -> stays "p1", updates tag
		//   "z"           -> owned by "p1"
		svcTree := New(svcCfg{project: "", tag: "root"}, 5, seqOf(
			kv[svcCfg]{"a", svcCfg{project: "p1", tag: "a"}},
			kv[svcCfg]{"a/b/sub", svcCfg{project: "p2", tag: "sub"}},
			kv[svcCfg]{"a/b/unowned", svcCfg{project: "", tag: "none"}},
			kv[svcCfg]{"a/sib", svcCfg{project: "p1", tag: "sib"}},
			kv[svcCfg]{"z", svcCfg{project: "p1", tag: "z"}},
		), nil)

		projTrees := map[string]*Tree[string]{
			"p1": New("p1:@root", 4, seqOf(
				kv[string]{"a/b/unowned/ignored", "p1:ignored"}, // must be ignored because unowned
				kv[string]{"a/deep/leaf", "p1:deep"},
				kv[string]{"a/sib/realm", "p1:sib-realm"},
				kv[string]{"z", "p1:z-realm"},
			), nil),
			"p2": New("p2:@root", 2, seqOf(
				// "a" is not owned by p2, but inherited when p2 is mounted at "a/b/sub"!
				kv[string]{"a", "p2:ancestor-realm"},
				// SetTree at "a/b/sub" lands mid-edge on "a/b/sub/c/d"!
				kv[string]{"a/b/sub/c/d", "p2:cd-realm"},
			), nil),
		}

		// Legacy reader tree:
		//   "a/rdr" -> true (pruned, so "a/rdr/redundant" is skipped, while "a/sib" is unaffected)
		//   "a/b/sub" -> true (pruned, but "a/b/sub/c/d" still sees hasReader=true!)
		//   "z/rdr" -> true (visited after earlier Prune calls unwound)
		rdrTree := New(false, 4, seqOf(
			kv[bool]{"a/rdr", true},
			kv[bool]{"a/rdr/redundant/child", true},
			kv[bool]{"a/b/sub", true},
			kv[bool]{"z/rdr", true},
		), nil)

		svc := NewTrack(svcTree)
		proj := NewTrack[string](nil)
		rdr := NewTrack(rdrTree)

		type combined struct {
			Path      string
			Project   string
			Tag       string
			Realm     string
			HasReader bool
		}
		var out []combined
		for p := range Walk("", svc, proj, rdr) {
			sc := svc.Value()
			proj.SetTree(projTrees[sc.project])
			hr := rdr.Value()
			if hr {
				rdr.Prune()
			}
			out = append(out, combined{
				Path:      p,
				Project:   sc.project,
				Tag:       sc.tag,
				Realm:     proj.Value(),
				HasReader: hr,
			})
		}

		assert.That(t, out, should.Match([]combined{
			{"", "", "root", "", false},
			{"a", "p1", "a", "p1:@root", false},
			{"a/b/sub", "p2", "sub", "p2:ancestor-realm", true},
			{"a/b/sub/c/d", "p2", "sub", "p2:cd-realm", true},
			{"a/b/unowned", "", "none", "", false},
			{"a/deep/leaf", "p1", "a", "p1:deep", false},
			{"a/rdr", "p1", "a", "p1:@root", true},
			{"a/sib", "p1", "sib", "p1:@root", false},
			{"a/sib/realm", "p1", "sib", "p1:sib-realm", false},
			{"z", "p1", "z", "p1:z-realm", false},
			{"z/rdr", "p1", "z", "p1:z-realm", true},
		}))
	})

	t.Run("SetTree to zero-value tree and tree missing current branch", func(t *testing.T) {
		driver := New("root", 3, seqOf(
			kv[string]{"a", "A"},
			kv[string]{"a/b/sub", "SUB"},
			kv[string]{"a/sib", "SIB"},
		), nil)
		// Has an ancestor value at "a" and a sibling "a/x", but nothing under "a/b".
		mounted := New("m:@root", 2, seqOf(
			kv[string]{"a", "m:a"},
			kv[string]{"a/x", "m:ax"},
		), nil)
		var zeroTree Tree[string]

		dTrack := driver.Track()
		mTrack := NewTrack[string](nil)
		var got []string
		for p := range Walk("", dTrack, mTrack) {
			switch p {
			case "a":
				mTrack.SetTree(&zeroTree)
			case "a/b/sub":
				mTrack.SetTree(mounted)
			}
			got = append(got, fmt.Sprintf("%s=%s/%s", p, dTrack.Value(), mTrack.Value()))
		}
		assert.That(t, got, should.Match([]string{
			"=root/",
			"a=A/",
			"a/b/sub=SUB/m:a",
			"a/sib=SIB/",
		}))
	})

	t.Run("large tree walk", func(t *testing.T) {
		var items []kv[int]
		for g := range 90 {
			items = append(items,
				kv[int]{fmt.Sprintf("g%02d/a", g), g*2 + 1},
				kv[int]{fmt.Sprintf("g%02d/b", g), g*2 + 2},
			)
		}
		tr := New(0, len(items), seqOf(items...), nil)

		track := NewTrack(tr)
		count := 0
		var prev string
		for s := range Walk("", track) {
			if count > 0 {
				assert.That(t, normPath(prev) < normPath(s), should.BeTrue)
			}
			prev = s
			count++
		}
		// 1 root + 180 leaves (90 unconfigured LCAs skipped) = 181 yielded paths.
		assert.That(t, count, should.Equal(181))
	})
}
