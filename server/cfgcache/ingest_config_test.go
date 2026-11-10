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
	"iter"
	"maps"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	cfgcachecommon "go.chromium.org/luci/server/cfgcache/common"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

func TestIngestConfigRegistrationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		register func(reg *Registry)
		wantErr  string
	}{
		{
			name: "empty ID",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					Store: &noopStore[*durationpb.Duration]{},
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `ID must not be empty`,
		},
		{
			name: "negative TransformVersion",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:               "svc",
					Store:            &noopStore[*durationpb.Duration]{},
					Files:            []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					TransformVersion: -1,
				}).Register(reg)
			},
			wantErr: `TransformVersion must not be negative`,
		},
		{
			name: "ToMemoryFunc.asGeneric error propagates",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration, int64]{
					ID:    "svc",
					Store: &noopStore[*durationpb.Duration]{},
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `ToMemory must be set when StoreMsg and InMemory types differ`,
		},
		{
			name: "NoMemoryRefresh with ToMemoryMaxVersion set",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration, NoMemoryRefresh]{
					ID:                 "svc",
					Store:              &noGetNoopStore[*durationpb.Duration]{},
					Files:              []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					ToMemoryMaxVersion: 1,
				}).Register(reg)
			},
			wantErr: `ToMemoryMinVersion and ToMemoryMaxVersion must not be set when InMemory is NoMemoryRefresh`,
		},
		{
			name: "negative ToMemoryMinVersion",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:                 "svc",
					Store:              &noopStore[*durationpb.Duration]{},
					Files:              []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					ToMemoryMinVersion: -1,
				}).Register(reg)
			},
			wantErr: `ToMemoryMinVersion must not be negative`,
		},
		{
			name: "negative ToMemoryMaxVersion",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:                 "svc",
					Store:              &noopStore[*durationpb.Duration]{},
					Files:              []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					ToMemoryMaxVersion: -1,
				}).Register(reg)
			},
			wantErr: `ToMemoryMaxVersion must not be negative`,
		},
		{
			name: "ToMemoryMinVersion > TransformVersion",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:                 "svc",
					Store:              &noopStore[*durationpb.Duration]{},
					Files:              []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					TransformVersion:   1,
					ToMemoryMinVersion: 2,
					ToMemoryMaxVersion: 2,
				}).Register(reg)
			},
			wantErr: `ToMemoryMinVersion (2) must be <= TransformVersion (1)`,
		},
		{
			name: "ToMemoryMaxVersion < TransformVersion",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:               "svc",
					Store:            &noopStore[*durationpb.Duration]{},
					Files:            []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
					TransformVersion: 1,
				}).Register(reg)
			},
			wantErr: `ToMemoryMaxVersion (0) must be >= TransformVersion (1)`,
		},
		{
			name: "ungettable Store with InMemory != NoMemoryRefresh",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:    "svc",
					Store: &noGetNoopStore[*durationpb.Duration]{},
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `in-memory refresh requires a GettableStore, got *cfgcache.noGetNoopStore`,
		},
		{
			name: "duplicate ID",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:    "dup",
					Store: &noopStore[*durationpb.Duration]{},
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "a.cfg"}},
				}).Register(reg)
				(&PassthroughProjectPipeline[*durationpb.Duration]{
					ID:    "dup",
					Store: &noopStore[*durationpb.Duration]{},
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "b.cfg"}},
				}).Register(reg)
			},
			wantErr: `duplicate ID "dup"`,
		},
		{
			name: "invalid RegexFile pattern",
			register: func(reg *Registry) {
				(&ServicePipeline[*durationpb.Duration, NoMemoryRefresh]{
					ID:    "svc",
					Store: &noopStore[*durationpb.Duration]{},
					Files: []File{
						&RegexFile[*durationpb.Duration]{PathRegex: `[unclosed`},
					},
					TransformVersion: 1,
					Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
						return &durationpb.Duration{}, nil
					},
				}).Register(reg)
			},
			wantErr: `bad PathRegex "[unclosed"`,
		},
		{
			name: "unresolvable variable in ServiceName",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:          "svc",
					Store:       &noopStore[*durationpb.Duration]{},
					ServiceName: "${bad_svc}",
					Files:       []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `bad serviceNamePattern: no placeholder named "bad_svc" is registered`,
		},
		{
			name: "unresolvable variable in Files path",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:    "svc",
					Store: &noopStore[*durationpb.Duration]{},
					Files: []File{&SingleFile[*durationpb.Duration]{Path: "${bad_path}.cfg"}},
				}).Register(reg)
			},
			wantErr: `Files[0]: bad path template: no placeholder named "bad_path" is registered`,
		},
		{
			name: "invalid ServiceName",
			register: func(reg *Registry) {
				(&PassthroughServicePipeline[*durationpb.Duration]{
					ID:          "svc",
					Store:       &noopStore[*durationpb.Duration]{},
					ServiceName: "Bad/Service",
					Files:       []File{&SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
				}).Register(reg)
			},
			wantErr: `bad ServiceName: invalid service name "Bad/Service"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, ts := TestingContext(t.Context())
			tc.register(ts.Registry)
			assert.ErrIsLike(t, ts.Initialize(ctx), tc.wantErr)
		})
	}
}

func TestTestingStateSetConfig(t *testing.T) {
	t.Parallel()

	cs := config.Set("services/my-app")

	cases := []struct {
		name    string
		path    string
		val     any
		wantGet string
	}{
		{
			name:    "string",
			path:    "str.txt",
			val:     "hello",
			wantGet: "hello",
		},
		{
			name:    "[]byte",
			path:    "raw.bin",
			val:     []byte("world"),
			wantGet: "world",
		},
		{
			name:    "proto.Message",
			path:    "msg.cfg",
			val:     &durationpb.Duration{Seconds: 42},
			wantGet: "seconds",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, ts := TestingContext(t.Context())
			ts.SetConfig(cs, tc.path, tc.val)
			assert.That(t, ts.CfgFiles[cs][tc.path], should.ContainSubstring(tc.wantGet))
		})
	}

	t.Run("nil deletes existing config", func(t *testing.T) {
		t.Parallel()
		_, ts := TestingContext(t.Context())
		ts.SetConfig(cs, "str.txt", "hello")
		ts.SetConfig(cs, "str.txt", nil)
		_, ok := ts.CfgFiles[cs]["str.txt"]
		assert.That(t, ok, should.BeFalse)
	})
}

func TestRegistryLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("double Initialize returns error", func(t *testing.T) {
		t.Parallel()
		ctx, ts := TestingContext(t.Context())
		assert.NoErr(t, ts.Initialize(ctx))
		assert.ErrIsLike(t, ts.Initialize(ctx), "registry already initialized")
	})

	t.Run("stateless descriptors can be registered across multiple registries", func(t *testing.T) {
		t.Parallel()
		f := &SingleFile[*durationpb.Duration]{Path: "${appid}.cfg"}
		svcCfg := &PassthroughServicePipeline[*durationpb.Duration]{ID: "svc", Store: &noopStore[*durationpb.Duration]{}, Files: []File{f}}
		projCfg := &PassthroughProjectPipeline[*durationpb.Duration]{ID: "proj", Store: &noopStore[*durationpb.Duration]{}, Files: []File{f}}

		ctx1, ts1 := TestingContext(t.Context())
		svcCfg.Register(ts1.Registry)
		projCfg.Register(ts1.Registry)
		assert.NoErr(t, ts1.Initialize(ctx1))

		ctx2, ts2 := TestingContext(t.Context())
		svcCfg.Register(ts2.Registry)
		projCfg.Register(ts2.Registry)
		assert.NoErr(t, ts2.Initialize(ctx2))
	})

	t.Run("NoMemoryRefresh Register returns nil handle", func(t *testing.T) {
		t.Parallel()
		_, ts := TestingContext(t.Context())
		f := &SingleFile[*durationpb.Duration]{Path: "nomem.cfg"}
		noMemSvc := (&ServicePipeline[*durationpb.Duration, NoMemoryRefresh]{ID: "svc", Files: []File{f}}).Register(ts.Registry)
		noMemProj := (&ProjectPipeline[*durationpb.Duration, NoMemoryRefresh]{ID: "proj", Files: []File{f}}).Register(ts.Registry)
		assert.That(t, noMemSvc, should.Equal[*RegisteredService[NoMemoryRefresh]](nil))
		assert.That(t, noMemProj, should.Equal[*RegisteredProjects[NoMemoryRefresh]](nil))
	})

	t.Run("String formatting", func(t *testing.T) {
		t.Parallel()
		_, ts := TestingContext(t.Context())
		f := &SingleFile[*durationpb.Duration]{Path: "neat.cfg"}
		svcDesc := &PassthroughServicePipeline[*durationpb.Duration]{ID: "svc-neat", Files: []File{f}}
		projDesc := &PassthroughProjectPipeline[*durationpb.Duration]{ID: "proj-multi", Files: []File{f}}
		assert.That(t, svcDesc.String(), should.Equal(`ServicePipeline{ID: "svc-neat", ServiceName: "${appid}"}`))
		assert.That(t, projDesc.String(), should.Equal(`ProjectPipeline{ID: "proj-multi"}`))
		rSvc := svcDesc.Register(ts.Registry)
		rProj := projDesc.Register(ts.Registry)

		cases := []struct {
			name string
			got  string
			want string
		}{
			{
				name: "ServicePipeline",
				got:  svcDesc.String(),
				want: `ServicePipeline{ID: "svc-neat", ServiceName: "${appid}"}`,
			},
			{
				name: "ProjectPipeline",
				got:  projDesc.String(),
				want: `ProjectPipeline{ID: "proj-multi"}`,
			},
			{
				name: "RegisteredService",
				got:  rSvc.String(),
				want: `RegisteredService[ServicePipeline{ID: "svc-neat", ServiceName: "${appid}"}]`,
			},
			{
				name: "RegisteredProjects",
				got:  rProj.String(),
				want: `RegisteredProjects[ProjectPipeline{ID: "proj-multi"}]`,
			},
			{
				name: "nil RegisteredService",
				got:  (*RegisteredService[*durationpb.Duration])(nil).String(),
				want: "RegisteredService[nil]",
			},
			{
				name: "nil RegisteredProjects",
				got:  (*RegisteredProjects[*durationpb.Duration])(nil).String(),
				want: "RegisteredProjects[nil]",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				assert.That(t, tc.got, should.Equal(tc.want))
			})
		}
	})
}

func TestMultiFile(t *testing.T) {
	t.Parallel()

	t.Run(`single`, func(t *testing.T) {
		t.Parallel()

		ctx, ts := TestingContext(t.Context())
		(&PassthroughProjectPipeline[*durationpb.Duration]{
			ID:    "the-pipeline",
			Store: &noopStore[*durationpb.Duration]{},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "afile"},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))
		assert.That(t, ts.Registry.ingestConfigs["the-pipeline"].multiFile(), should.BeFalse)
	})

	t.Run(`regex`, func(t *testing.T) {
		t.Parallel()

		ctx, ts := TestingContext(t.Context())
		(&ProjectPipeline[*durationpb.Duration, NoMemoryRefresh]{
			ID:               "the-pipeline",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, nil
			},
			Files: []File{
				&RegexFile[*durationpb.Duration]{PathRegex: "afile.*"},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))
		assert.That(t, ts.Registry.ingestConfigs["the-pipeline"].multiFile(), should.BeTrue)
	})

	t.Run(`multi-single`, func(t *testing.T) {
		t.Parallel()

		ctx, ts := TestingContext(t.Context())
		(&ProjectPipeline[*durationpb.Duration, NoMemoryRefresh]{
			ID:               "the-pipeline",
			Store:            &noopStore[*durationpb.Duration]{},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *Snapshot) (*durationpb.Duration, error) {
				return nil, nil
			},
			Files: []File{
				&SingleFile[*durationpb.Duration]{Path: "afile"},
				&SingleFile[*durationpb.Duration]{Path: "bfile"},
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))
		assert.That(t, ts.Registry.ingestConfigs["the-pipeline"].multiFile(), should.BeTrue)
	})
}

type mockGettableStore struct {
	noopStore[*durationpb.Duration]
	metas map[config.Set]*cfgcachepb.Meta
	msgs  map[config.Set]*durationpb.Duration
}

func (s *mockGettableStore) LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]*cfgcachepb.Meta, error) {
	return maps.Clone(s.metas), nil
}

func (s *mockGettableStore) Put(ctx context.Context, pipelineID string, cs config.Set, m *cfgcachepb.Meta, out *durationpb.Duration) error {
	if s.metas == nil {
		s.metas = make(map[config.Set]*cfgcachepb.Meta)
		s.msgs = make(map[config.Set]*durationpb.Duration)
	}
	s.metas[cs] = m
	s.msgs[cs] = out
	return nil
}

func (s *mockGettableStore) Delete(ctx context.Context, pipelineID string, sets []config.Set) error {
	for _, cs := range sets {
		delete(s.metas, cs)
		delete(s.msgs, cs)
	}
	return nil
}

func (s *mockGettableStore) Get(ctx context.Context, pipelineID string, sets map[config.Set]*cfgcachepb.Meta) iter.Seq2[*cfgcachecommon.StoredConfig[*durationpb.Duration], error] {
	return func(yield func(*cfgcachecommon.StoredConfig[*durationpb.Duration], error) bool) {
		for cs := range sets {
			m, ok := s.metas[cs]
			if !ok {
				continue
			}
			if !yield(&cfgcachecommon.StoredConfig[*durationpb.Duration]{
				ConfigSet: cs,
				Meta:      m,
				Config:    s.msgs[cs],
			}, nil) {
				return
			}
		}
	}
}

func TestStateRecordSnapshot(t *testing.T) {
	t.Parallel()

	ctx, ts := TestingContext(t.Context())
	s := &mockGettableStore{}

	var failP1 atomic.Bool
	(&ProjectPipeline[*durationpb.Duration, int64]{
		ID:                 "proj",
		Store:              s,
		Files:              []File{&SingleFile[*durationpb.Duration]{Path: "proj.cfg"}},
		TransformVersion:   2,
		ToMemoryMinVersion: 2,
		ToMemoryMaxVersion: 3,
		ToMemory: func(ctx context.Context, cs config.Set, in *durationpb.Duration) (int64, error) {
			if cs == "projects/p1" && failP1.Load() {
				return 0, errors.New("p1 bad transform")
			}
			return in.Seconds, nil
		},
	}).Register(ts.Registry)
	assert.NoErr(t, ts.Initialize(ctx))

	ic := ts.Registry.ingestConfigs["proj"]

	// 1. Initial load succeeds for p1.
	meta1 := &cfgcachepb.Meta{TransformVersion: 2, InputHash: "h1"}
	assert.NoErr(t, s.Put(ctx, "proj", "projects/p1", meta1, &durationpb.Duration{Seconds: 10}))
	assert.NoErr(t, ts.RefreshInMemory(ctx))

	snap := ic.state.snapshot()
	assert.NoErr(t, snap.lastRefreshErr)
	assert.That(t, snap.acceptedState, should.Match(map[config.Set]*cfgcachepb.Meta{
		"projects/p1": meta1,
	}))
	assert.That(t, len(snap.configSetErrors), should.Equal(0))

	// 2. Subsequent refresh has a ToMemory error on p1 and a version mismatch on p2.
	failP1.Store(true)
	meta1v2 := &cfgcachepb.Meta{TransformVersion: 2, InputHash: "h1-v2"}
	meta2bad := &cfgcachepb.Meta{TransformVersion: 5, InputHash: "h2-v5"}
	assert.NoErr(t, s.Put(ctx, "proj", "projects/p1", meta1v2, &durationpb.Duration{Seconds: 20}))
	assert.NoErr(t, s.Put(ctx, "proj", "projects/p2", meta2bad, &durationpb.Duration{Seconds: 30}))
	assert.NoErr(t, ts.RefreshInMemory(ctx))

	snap = ic.state.snapshot()
	assert.NoErr(t, snap.lastRefreshErr)
	assert.That(t, snap.acceptedState, should.Match(map[config.Set]*cfgcachepb.Meta{
		"projects/p1": meta1,
	}))
	assert.ErrIsLike(t, snap.configSetErrors["projects/p1"], "consuming projects/p1: p1 bad transform")
	assert.ErrIsLike(t, snap.configSetErrors["projects/p2"], "proj[projects/p2]: transform version is 5 > max(3)")

	// 3. Clearing the error and deleting p2 clears configSetErrors and updates acceptedState.
	failP1.Store(false)
	assert.NoErr(t, s.Delete(ctx, "proj", []config.Set{"projects/p2"}))
	assert.NoErr(t, ts.RefreshInMemory(ctx))

	snap = ic.state.snapshot()
	assert.That(t, snap.acceptedState, should.Match(map[config.Set]*cfgcachepb.Meta{
		"projects/p1": meta1v2,
	}))
	assert.That(t, len(snap.configSetErrors), should.Equal(0))
}
