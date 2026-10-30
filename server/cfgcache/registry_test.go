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

package cfgcache

import (
	"context"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"
)

func TestValidationAndTransform(t *testing.T) {
	t.Parallel()

	initRules := func(t *testing.T, reg *Registry) *validation.RuleSet {
		t.Helper()
		v := &vars.VarSet{}
		v.Register("appid", func(context.Context) (string, error) { return "my-app", nil })
		rules := &validation.RuleSet{Vars: v}
		assert.NoErr(t, reg.Initialize(t.Context(), v))
		reg.InstallValidationRules(rules)
		return rules
	}

	t.Run("SingleFile Path ignores magical pattern forms", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}
		(&ServicePipeline[*durationpb.Duration]{
			ID:    "svc-colon",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{&SingleFile[*durationpb.Duration]{
				// NOTE: This matches the file called `regex:literal.cfg` in order
				// to prove that we're escaping the magical pattern prefix stuff.
				Path: "regex:literal.cfg",
			}},
		}).Register(reg)

		rules := initRules(t, reg)

		valCtx := &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfig(valCtx, "services/my-app", "regex:literal.cfg", []byte(`seconds: 7`)))
		assert.NoErr(t, valCtx.Finalize())

		// Assert that a BAD file content in the path actually yields an error
		valCtx = &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfig(valCtx, "services/my-app", "regex:literal.cfg", []byte(`nerpnerp: 7`)))
		assert.ErrIsLike(t, valCtx.Finalize(), "failed to unmarshal")

		// Unmatched path does not trigger validation on "regex:literal.cfg".
		valCtx = &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfig(valCtx, "services/my-app", "other.cfg", []byte(`ignore me`)))
		assert.NoErr(t, valCtx.Finalize())
	})

	t.Run("SingleFile zero default", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}

		noDefault := &SingleFile[string]{Path: "label.txt"}

		(&ProjectPipeline[*durationpb.Duration]{
			ID:               "proj-defaults",
			Store:            &noopStore[*durationpb.Duration]{},
			Files:            []File{noDefault},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				val := noDefault.Get(snap)
				assert.That(t, val, should.Equal(""))
				return durationpb.New(3 * time.Second), nil
			},
		}).Register(reg)

		rules := initRules(t, reg)

		valCtx := &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfigSet(valCtx, "projects/chromium", nil))
		assert.NoErr(t, valCtx.Finalize())
	})

	t.Run("SingleFile NewDefault", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}

		// *string is a silly type, but we want something nillable which isn't a
		// proto.
		defaulted := &SingleFile[*string]{
			Path:       "label.txt",
			NewDefault: func() *string { ret := "banana"; return &ret },
			Unmarshal:  func(raw string) (*string, error) { return &raw, nil },
		}

		(&ProjectPipeline[*durationpb.Duration]{
			ID:               "proj-defaults",
			Store:            &noopStore[*durationpb.Duration]{},
			Files:            []File{defaulted},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				valA := defaulted.Get(snap)
				valB := defaulted.Get(snap)
				assert.That(t, *valA, should.Equal("banana"))
				assert.That(t, *valB, should.Equal("banana"))
				assert.That(t, valA, should.NotEqual(valB))
				return durationpb.New(3 * time.Second), nil
			},
		}).Register(reg)

		rules := initRules(t, reg)

		valCtx := &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfigSet(valCtx, "projects/chromium", nil))
		assert.NoErr(t, valCtx.Finalize())
	})

	t.Run("RegexFile aggregates matching files and reports unmarshal errors", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}

		projShards := &RegexFile[int]{
			PathRegex: `silly/.*\.txt`,
			Unmarshal: strconv.Atoi,
		}
		var gotSum int
		(&ProjectPipeline[*timestamppb.Timestamp]{
			ID:               "proj-silly",
			Store:            &noopStore[*timestamppb.Timestamp]{},
			Files:            []File{projShards},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*timestamppb.Timestamp, error) {
				gotSum = 0
				for _, v := range projShards.Get(snap) {
					gotSum += v
				}
				return &timestamppb.Timestamp{Seconds: int64(gotSum)}, nil
			},
		}).Register(reg)

		rules := initRules(t, reg)

		// Try a couple silly configs.
		valCtx := &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfigSet(valCtx, "projects/chromium", map[string][]byte{
			"silly/s1.txt": []byte("10"),
			"silly/s2.txt": []byte("20"),
		}))
		assert.NoErr(t, valCtx.Finalize())
		assert.That(t, gotSum, should.Equal(30))

		// Now try with something that fails unmarshal.
		valCtx = &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfigSet(valCtx, "projects/chromium", map[string][]byte{
			"silly/bad.txt": []byte("not-an-int"),
		}))
		assert.ErrIsLike(t, valCtx.Finalize(), `in "silly/bad.txt": failed to unmarshal`)
	})

	t.Run("RegexFile patterns operate independently (even for the same files)", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}

		strLike := &RegexFile[string]{
			PathRegex: `over/a.*`,
		}
		intLike := &RegexFile[int]{
			PathRegex: `over/a.*`,
			Unmarshal: strconv.Atoi,
		}
		(&ServicePipeline[*durationpb.Duration]{
			ID:               "svc-overlap",
			Store:            &noopStore[*durationpb.Duration]{},
			Files:            []File{strLike, intLike},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				assert.That(t, strLike.Get(snap)["over/ab"], should.Equal("10"))
				assert.That(t, intLike.Get(snap)["over/ab"], should.Equal(10))
				return &durationpb.Duration{}, nil
			},
		}).Register(reg)

		rules := initRules(t, reg)

		valCtx := &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfig(valCtx, "services/my-app", "over/ab", []byte("10")))
		assert.NoErr(t, valCtx.Finalize())
	})

	t.Run("fatal Transform error propagates directly", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}

		f := &SingleFile[*durationpb.Duration]{Path: "proj.cfg"}
		(&ProjectPipeline[*durationpb.Duration]{
			ID:    "proj-fatal",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{f},
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, errors.New("fatal transform failure")
			},
		}).Register(reg)

		rules := initRules(t, reg)

		valCtx := &validation.Context{Context: t.Context()}
		err := rules.ValidateConfig(valCtx, "projects/chromium", "proj.cfg", []byte(`seconds: 1`))
		assert.ErrIsLike(t, err, "fatal transform failure")
	})

	t.Run("Multiple Files", func(t *testing.T) {
		t.Parallel()
		reg := &Registry{}

		var a, b time.Duration

		aHandle := &SingleFile[*durationpb.Duration]{Path: "a.cfg"}
		bHandle := &SingleFile[*durationpb.Duration]{Path: "b.cfg"}

		(&ServicePipeline[*durationpb.Duration]{
			ID:               "svc-colon",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				a = aHandle.Get(snap).AsDuration()
				b = bHandle.Get(snap).AsDuration()
				return nil, nil
			},
			Files: []File{aHandle, bHandle},
		}).Register(reg)

		rules := initRules(t, reg)

		valCtx := &validation.Context{Context: t.Context()}
		assert.NoErr(t, rules.ValidateConfigSet(valCtx, "services/my-app", map[string][]byte{
			"a.cfg":     []byte(`seconds: 10`),
			"b.cfg":     []byte(`seconds: 100`),
			"other.cfg": []byte(`ignoreme`),
		}))
		assert.NoErr(t, valCtx.Finalize())

		assert.That(t, a, should.Equal(10*time.Second))
		assert.That(t, b, should.Equal(100*time.Second))
	})
}

func TestCompileFileRegex(t *testing.T) {
	t.Parallel()

	t.Run(`single`, func(t *testing.T) {
		t.Parallel()

		ctx, ts := TestingContext(t.Context())
		(&ServicePipeline[*durationpb.Duration]{
			ID:    "svc-pipeline",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "afile"},
			},
		}).Register(ts.Registry)
		(&ProjectPipeline[*durationpb.Duration]{
			ID:    "proj-pipeline",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "afile"},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		check.Loosely(t, ts.Registry.projectFileRegex, should.BeNil)
		check.Loosely(t, ts.Registry.serviceFileRegexes["services/my-app"].String(),
			should.Match("^(?:(?:afile))$"))
		check.Loosely(t, ts.Registry.singleProjectFile, should.Equal("afile"))
	})

	t.Run(`multi single-file`, func(t *testing.T) {
		t.Parallel()

		ctx, ts := TestingContext(t.Context())
		(&ProjectPipeline[*durationpb.Duration]{
			ID:               "the-pipeline",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, nil
			},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "cf.ile"},
				&SingleFile[*durationpb.Duration]{Path: "afi*le"},
				&SingleFile[*durationpb.Duration]{Path: "qfile"},
				&SingleFile[*durationpb.Duration]{Path: "afi*le"},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		check.That(t, ts.Registry.projectFileRegex.String(), should.Equal(
			`^(?:(?:afi\*le)|(?:cf\.ile)|(?:qfile))$`))
		check.Loosely(t, ts.Registry.serviceFileRegexes, should.BeEmpty)
		check.Loosely(t, ts.Registry.singleProjectFile, should.Equal(""))
	})

	t.Run(`mixed`, func(t *testing.T) {
		t.Parallel()

		ctx, ts := TestingContext(t.Context())
		(&ProjectPipeline[*durationpb.Duration]{
			ID:               "the-pipeline",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, nil
			},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "cf.ile"},
				&RegexFile[*durationpb.Duration]{PathRegex: "afile.+"},
				&SingleFile[*durationpb.Duration]{Path: "qfile"},
				&SingleFile[*durationpb.Duration]{Path: "afile"},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		check.That(t, ts.Registry.projectFileRegex.String(), should.Equal(
			`^(?:(?:afile)|(?:afile.+)|(?:cf\.ile)|(?:qfile))$`))
		check.Loosely(t, ts.Registry.serviceFileRegexes, should.BeEmpty)
		check.Loosely(t, ts.Registry.singleProjectFile, should.Equal(""))
	})
}
