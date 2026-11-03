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
	"crypto/sha256"
	"slices"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
)

func makeConfig(cs config.Set, path, content string) (ret config.Config) {
	ret.ConfigSet = cs
	ret.Path = path
	ret.Content = content
	ret.ContentSize = int64(len(content))
	sum := sha256.Sum256([]byte(content))
	ret.ContentHash = string(sum[:])
	return ret
}

func TestFlatSnapshot(t *testing.T) {
	t.Parallel()

	snap := flatSnapshotFromMap(map[string]config.Config{
		"some/file":    makeConfig(config.MustProjectSet("p1"), "some/file", "hello"),
		"another/file": makeConfig(config.MustProjectSet("p1"), "another/file", "another-hello"),
	})
	assert.That(t, snap.paths(), should.Match([]string{"another/file", "some/file"}))

	assert.That(t, snap.get("some/file").Content, should.Equal("hello"))
	assert.That(t, snap.get("another/file").Content, should.Equal("another-hello"))
	assert.Loosely(t, snap.get("missing"), should.BeNil)
}

func TestLoadedConfig(t *testing.T) {
	t.Parallel()

	snap := loadedConfigFromConfigs([]config.Config{
		makeConfig(config.MustProjectSet("p1"), "some/file", "hello1"),
		makeConfig(config.MustProjectSet("p1"), "another/file", "another-hello1"),
		makeConfig(config.MustProjectSet("p2"), "some/file", "hello2"),
		makeConfig(config.MustProjectSet("p2"), "another/file", "another-hello2"),
		makeConfig(config.MustServiceSet("s1"), "something", "flat"),
		makeConfig(config.MustServiceSet("s2"), "something-else", "flat-else"),
	})

	assert.That(t, slices.Sorted(func(yield func(string) bool) {
		for cs := range snap.projectConfigs {
			yield(string(cs))
		}
	}), should.Match([]string{"projects/p1", "projects/p2"}))

	assert.That(t, snap["projects/p1"], should.Match(flatSnapshotFromMap(map[string]config.Config{
		"some/file":    makeConfig(config.MustProjectSet("p1"), "some/file", "hello1"),
		"another/file": makeConfig(config.MustProjectSet("p1"), "another/file", "another-hello1"),
	})))

	assert.That(t, snap["projects/p2"], should.Match(flatSnapshotFromMap(map[string]config.Config{
		"some/file":    makeConfig(config.MustProjectSet("p2"), "some/file", "hello2"),
		"another/file": makeConfig(config.MustProjectSet("p2"), "another/file", "another-hello2"),
	})))

	assert.That(t, slices.Sorted(func(yield func(string) bool) {
		for cs := range snap.serviceConfigs("s1") {
			yield(string(cs))
		}
	}), should.Match([]string{"services/s1"}))

	assert.That(t, slices.Sorted(func(yield func(string) bool) {
		for cs := range snap.serviceConfigs("s2") {
			yield(string(cs))
		}
	}), should.Match([]string{"services/s2"}))

	assert.That(t, snap["services/s1"], should.Match(flatSnapshotFromMap(map[string]config.Config{
		"something": makeConfig(config.MustServiceSet("s1"), "something", "flat"),
	})))

	assert.That(t, snap["services/s2"], should.Match(flatSnapshotFromMap(map[string]config.Config{
		"something": makeConfig(config.MustServiceSet("s2"), "something-else", "flat-else"),
	})))
}
