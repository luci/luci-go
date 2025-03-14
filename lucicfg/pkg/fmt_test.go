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

package pkg

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/lucicfg/buildifier"
)

func TestStandardFormatter(t *testing.T) {
	t.Parallel()

	f := standardFormatter([]*FmtRule{
		{
			Paths:                 []string{".", "a"},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"root"},
		},
		{
			Paths:                 []string{"a/b"},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"a_b"},
		},
		{
			Paths:                 []string{"a/bc"},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"a_bc"},
		},
		{
			Paths:                 []string{"a/b/c"},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"a_b_c"},
		},
		{
			Paths:            []string{"a/b/c/disable"},
			SortFunctionArgs: false,
		},
		{
			Paths:                 []string{"a/d"},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"a_d"},
		},
	})

	rule := func(path string) string {
		rw, err := f.RewriterForPath(context.Background(), path)
		assert.NoErr(t, err)
		if rw == nil {
			return ""
		}
		for val := range rw.NamePriority {
			return val
		}
		panic("impossible")
	}

	cases := []struct {
		path string
		res  string
	}{
		{"f.star", "root"},
		{"a/f.star", "root"},
		{"a/b/f.star", "a_b"},
		{"a/bf.star", "root"},
		{"a/bc/f.star", "a_bc"},
		{"a/b/c/f.star", "a_b_c"},
		{"a/b/c/disable/f.star", ""},
		{"a/d/c/f.star", "a_d"},
	}
	for _, cs := range cases {
		assert.That(t, rule(cs.path), should.Equal(cs.res), truth.Explain("path = %s", cs.path))
	}

	// A test without a root.
	f = standardFormatter([]*FmtRule{
		{
			Paths:                 []string{"a/b/c"},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"a_b_c"},
		},
	})
	assert.That(t, rule("a/b/c.star"), should.Equal(""))
}

func TestLegacyRulesToFmtRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		legacy string
		rules  []*FmtRule
		err    string
	}{
		{
			`rules {
				path: "p1"
				path: "p2"
				function_args_sort {
					arg: "arg1"
					arg: "arg2"
				}
			}
			rules {
				path: "p3"
				function_args_sort {
				}
			}
			rules {
				path: "p4"
			}
			`,
			[]*FmtRule{
				{
					Paths:                 []string{"p1", "p2"},
					SortFunctionArgs:      true,
					SortFunctionArgsOrder: []string{"arg1", "arg2"},
				},
				{
					Paths:                 []string{"p3"},
					SortFunctionArgs:      true,
					SortFunctionArgsOrder: []string{},
				},
				{
					Paths:            []string{"p4"},
					SortFunctionArgs: false,
				},
			},
			"",
		},
		{
			`rules {}`,
			nil,
			"rule #0: paths should not be empty",
		},
		{
			`rules {
				path: "p"
				path: "p"
			}`,
			nil,
			"rule #0: has duplicate paths",
		},
		{
			`rules {
				path: ""
			}`,
			nil,
			"rule #0: empty path",
		},
		{
			`rules {
				path: "abc/.."
			}`,
			nil,
			`rule #0: path "abc/.." is not in normal form (e.g. ".")`,
		},
		{
			`rules {
				path: "a"
			}
			rules {
				path: "a"
			}`,
			nil,
			`rule #1: path "a" was already specified in another rule`,
		},
		{
			`rules {
				path: "a"
				function_args_sort {
					arg: "arg"
					arg: "arg"
				}
			}`,
			nil,
			`rule #0: has duplicate args in function_args_sort`,
		},
		{
			`rules {
				path: "a"
				function_args_sort {
					arg: ""
				}
			}`,
			nil,
			`rule #0: empty arg in function_args_sort`,
		},
	}

	for _, cs := range cases {
		cfg := &buildifier.LucicfgFmtConfig{}
		assert.NoErr(t, prototext.Unmarshal([]byte(cs.legacy), cfg))

		rules, err := legacyRulesToFmtRules(cfg)
		if cs.err != "" {
			assert.That(t, err, should.ErrLike(cs.err), truth.Explain("%s", cs.legacy))
		} else {
			assert.That(t, rules, should.Match(cs.rules))
		}
	}
}

func TestLegacyFormatter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("OK", func(t *testing.T) {
		f := legacyFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `rules {
				path: "."
				function_args_sort {
					arg: "arg"
				}
			}`,
		}))
		assert.NoErr(t, f.CheckValid(ctx))

		rw, err := f.RewriterForPath(ctx, "some/path.star")
		assert.NoErr(t, err)
		assert.That(t, rw.NamePriority, should.Match(map[string]int{
			"arg": -1,
		}))
	})

	t.Run("Bad proto", func(t *testing.T) {
		f := legacyFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `not a valid proto`,
		}))
		assert.That(t, f.CheckValid(ctx), should.ErrLike("bad text proto"))
		_, err := f.RewriterForPath(ctx, "some/path.star")
		assert.That(t, err, should.ErrLike("bad text proto"))
	})

	t.Run("Incorrect rules", func(t *testing.T) {
		f := legacyFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `rules {}`,
		}))
		assert.That(t, f.CheckValid(ctx), should.ErrLike("bad formatting rules"))
		_, err := f.RewriterForPath(ctx, "some/path.star")
		assert.That(t, err, should.ErrLike("bad formatting rules"))
	})

	t.Run("Missing config", func(t *testing.T) {
		f := legacyFormatter(prepDisk(t, map[string]string{
			// Nothing.
		}))
		assert.NoErr(t, f.CheckValid(ctx))
		rw, err := f.RewriterForPath(ctx, "some/path.star")
		assert.NoErr(t, err)
		assert.Loosely(t, rw, should.BeNil)
	})
}

func TestLegacyCompatibleFormatter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	rules := []*FmtRule{
		{
			Paths:                 []string{"."},
			SortFunctionArgs:      true,
			SortFunctionArgsOrder: []string{"arg"},
		},
	}

	t.Run("OK: match", func(t *testing.T) {
		f := legacyCompatibleFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `rules {
				path: "."
				function_args_sort {
					arg: "arg"
				}
			}`,
		}), rules)
		assert.NoErr(t, f.CheckValid(ctx))

		rw, err := f.RewriterForPath(ctx, "some/path.star")
		assert.NoErr(t, err)
		assert.That(t, rw.NamePriority, should.Match(map[string]int{
			"arg": -1,
		}))
	})

	t.Run("OK: legacy only", func(t *testing.T) {
		f := legacyCompatibleFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `rules {
				path: "."
				function_args_sort {
					arg: "legacy"
				}
			}`,
		}), nil)
		assert.NoErr(t, f.CheckValid(ctx))

		rw, err := f.RewriterForPath(ctx, "some/path.star")
		assert.NoErr(t, err)
		assert.That(t, rw.NamePriority, should.Match(map[string]int{
			"legacy": -1,
		}))
	})

	t.Run("OK: new only", func(t *testing.T) {
		f := legacyCompatibleFormatter(prepDisk(t, map[string]string{
			// Empty.
		}), rules)
		assert.NoErr(t, f.CheckValid(ctx))

		rw, err := f.RewriterForPath(ctx, "some/path.star")
		assert.NoErr(t, err)
		assert.That(t, rw.NamePriority, should.Match(map[string]int{
			"arg": -1,
		}))
	})

	t.Run("Bad legacy proto", func(t *testing.T) {
		f := legacyCompatibleFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `not a valid proto`,
		}), rules)
		assert.That(t, f.CheckValid(ctx), should.ErrLike("bad text proto"))
		_, err := f.RewriterForPath(ctx, "some/path.star")
		assert.That(t, err, should.ErrLike("bad text proto"))
	})

	t.Run("Incorrect legacy rules", func(t *testing.T) {
		f := legacyCompatibleFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `rules {}`,
		}), rules)
		assert.That(t, f.CheckValid(ctx), should.ErrLike("bad formatting rules"))
		_, err := f.RewriterForPath(ctx, "some/path.star")
		assert.That(t, err, should.ErrLike("bad formatting rules"))
	})

	t.Run("Mismatch", func(t *testing.T) {
		f := legacyCompatibleFormatter(prepDisk(t, map[string]string{
			".lucicfgfmtrc": `rules {
				path: "."
				function_args_sort {
					arg: "legacy"
				}
			}`,
		}), rules)
		assert.That(t, f.CheckValid(ctx), should.ErrLike("should be identical"))
		_, err := f.RewriterForPath(ctx, "some/path.star")
		assert.That(t, err, should.ErrLike("should be identical"))
	})
}
