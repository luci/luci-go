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
	"maps"
	"slices"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
)

func TestRegistryFetchConfigMeta(t *testing.T) {
	t.Parallel()

	setup := func(t testing.TB) (context.Context, *TestingState) {
		ctx, ts := TestingContext(t.Context())

		(&ProjectPipeline[*durationpb.Duration]{
			ID:               "proj",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, nil
			},
			Files: []File{
				&SingleFile[string]{
					Path: "spam",
				},
				&RegexFile[string]{
					PathRegex: "re(-spam)*",
				},
			},
		}).Register(ts.Registry)

		(&ServicePipeline[*durationpb.Duration]{
			ID:               "service",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, nil
			},
			Files: []File{
				&SingleFile[string]{
					Path: "spam",
				},
				&RegexFile[string]{
					PathRegex: "re(-spam)*",
				},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))
		return ctx, ts
	}

	t.Run(`empty`, func(t *testing.T) {
		t.Parallel()
		ctx, ts := setup(t)
		client := cfgclient.Client(ctx)

		got, err := ts.Registry.fetchConfigMeta(ctx, client)
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.BeEmpty)
	})

	t.Run(`service-only, partial`, func(t *testing.T) {
		t.Parallel()
		ctx, ts := setup(t)
		client := cfgclient.Client(ctx)

		ts.SetConfig("services/my-app", "unrelated", "bleh")
		ts.SetConfig("services/my-app", "spam", "egggs")
		// no re-spam

		got, err := ts.Registry.fetchConfigMeta(ctx, client)
		assert.NoErr(t, err)

		snap := got["services/my-app"]
		// "unrelated" was not fetched.
		assert.That(t, slices.Sorted(maps.Keys(snap)), should.Match([]string{"spam"}))
		spam := snap["spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("egggs")))
		assert.That(t, spam.Content, should.Equal(""))
	})

	t.Run(`full`, func(t *testing.T) {
		t.Parallel()
		ctx, ts := setup(t)
		client := cfgclient.Client(ctx)

		ts.SetConfig("services/my-app", "unrelated", "bleh")
		ts.SetConfig("services/my-app", "spam", "egggs")

		ts.SetConfig("projects/proj1", "spam", "egggs")
		ts.SetConfig("projects/proj1", "re-spam-spam", "eggs-eggs")

		ts.SetConfig("projects/proj2", "spam", "egggs")
		ts.SetConfig("projects/proj2", "re-spam-spam", "eggs-eggs")
		ts.SetConfig("projects/proj2", "re-spam-spam-spam", "eggs-eggs-eggs")

		ts.SetConfig("projects/proj3", "unrelated", "nope")

		got, err := ts.Registry.fetchConfigMeta(ctx, client)
		assert.NoErr(t, err)

		assert.That(t, slices.Sorted(maps.Keys(got)), should.Match([]config.Set{
			"projects/proj1",
			"projects/proj2",
			"services/my-app",
			// proj3 had nothing interesting
		}))

		snap := got["services/my-app"]
		assert.That(t, slices.Sorted(maps.Keys(snap)), should.Match([]string{"spam"}))
		spam := snap["spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("egggs")))
		assert.That(t, spam.Content, should.Equal(""))

		snap = got["projects/proj1"]
		assert.That(t, slices.Sorted(maps.Keys(snap)), should.Match([]string{
			"re-spam-spam",
			"spam",
		}))
		spam = snap["spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("egggs")))
		assert.That(t, spam.Content, should.Equal(""))
		spam = snap["re-spam-spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("eggs-eggs")))
		assert.That(t, spam.Content, should.Equal(""))

		snap = got["projects/proj2"]
		assert.That(t, slices.Sorted(maps.Keys(snap)), should.Match([]string{
			"re-spam-spam",
			"re-spam-spam-spam",
			"spam",
		}))
		spam = snap["spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("egggs")))
		assert.That(t, spam.Content, should.Equal(""))
		spam = snap["re-spam-spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("eggs-eggs")))
		assert.That(t, spam.Content, should.Equal(""))
		spam = snap["re-spam-spam-spam"]
		assert.Loosely(t, spam.ContentSize, should.Equal(len("eggs-eggs-eggs")))
		assert.That(t, spam.Content, should.Equal(""))
	})
}
