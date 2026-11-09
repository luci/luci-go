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

package memstore

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"

	cfgcachecommon "go.chromium.org/luci/server/cfgcache/common"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

func TestMemstore(t *testing.T) {
	t.Parallel()

	// Big dumb test - memstore is just intended for tests anyway.

	s := &Store[*durationpb.Duration]{}

	assert.NoErr(t, s.Put(t.Context(), "pipe1", "services/whatever", &cfgcachepb.Meta{
		InputHash:        "potato",
		TransformVersion: 7,
		TransformedSize:  100,
	}, durationpb.New(10*time.Second)))

	assert.NoErr(t, s.Put(t.Context(), "pipe2", "services/whatever", &cfgcachepb.Meta{
		InputHash:        "beans",
		TransformVersion: 70,
		TransformedSize:  200,
	}, durationpb.New(20*time.Second)))

	got, err := s.LoadMeta(t.Context(), "pipe1")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(map[config.Set]*cfgcachepb.Meta{
		"services/whatever": {
			InputHash:        "potato",
			TransformVersion: 7,
			TransformedSize:  100,
		},
	}))

	got, err = s.LoadMeta(t.Context(), "notreal")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(map[config.Set]*cfgcachepb.Meta(nil)))

	assert.NoErr(t, s.Delete(t.Context(), "pipe2", []config.Set{
		"services/whatever",
		"something/else",
	}))

	got, err = s.LoadMeta(t.Context(), "pipe2")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(map[config.Set]*cfgcachepb.Meta(nil)))

	got, err = s.LoadMeta(t.Context(), "pipe1")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(map[config.Set]*cfgcachepb.Meta{
		"services/whatever": {
			InputHash:        "potato",
			TransformVersion: 7,
			TransformedSize:  100,
		},
	}))

	sets := map[config.Set]*cfgcachepb.Meta{
		"services/whatever": {},
		"something/else":    {},
	}
	var pipe1Got []*cfgcachecommon.StoredConfig[*durationpb.Duration]
	for sc, err := range s.Get(t.Context(), "pipe1", sets) {
		assert.NoErr(t, err)
		pipe1Got = append(pipe1Got, sc)
	}
	assert.That(t, pipe1Got, should.Match([]*cfgcachecommon.StoredConfig[*durationpb.Duration]{{
		ConfigSet: "services/whatever",
		Meta: &cfgcachepb.Meta{
			InputHash:        "potato",
			TransformVersion: 7,
			TransformedSize:  100,
		},
		Config: &durationpb.Duration{Seconds: 10},
	}}))

	var pipe2Got []*cfgcachecommon.StoredConfig[*durationpb.Duration]
	for sc, err := range s.Get(t.Context(), "pipe2", sets) {
		assert.NoErr(t, err)
		pipe2Got = append(pipe2Got, sc)
	}
	assert.Loosely(t, pipe2Got, should.BeEmpty)
}
