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

package cfgcache_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/cfgcache/memstore"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

type mockStore[T proto.Message] struct {
	memstore.Store[T]
	loadMetaErr atomic.Pointer[error]
}

func (s *mockStore[T]) setLoadMetaErr(err error) {
	if err == nil {
		s.loadMetaErr.Store(nil)
	} else {
		s.loadMetaErr.Store(&err)
	}
}

func (s *mockStore[T]) LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]*cfgcachepb.Meta, error) {
	if errp := s.loadMetaErr.Load(); errp != nil {
		return nil, *errp
	}
	return s.Store.LoadMeta(ctx, pipelineID)
}

func TestRefresh(t *testing.T) {
	t.Parallel()

	t.Run("cfgcache.ServicePipeline and cfgcache.ProjectPipeline load, update, and delete configs", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		svc := (&cfgcache.ServicePipeline[*durationpb.Duration, int64]{
			ID:            "svc",
			Store:         s,
			Files:         []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "svc.cfg"}},
			MemoryDefault: func() int64 { return -1 },
			ToMemory: func(ctx context.Context, cs config.Set, in *durationpb.Duration) (int64, error) {
				return in.Seconds, nil
			},
		}).Register(ts.Registry)

		proj := (&cfgcache.ProjectPipeline[*durationpb.Duration, int64]{
			ID:    "proj",
			Store: s,
			Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "proj.cfg"}},
			ToMemory: func(ctx context.Context, cs config.Set, in *durationpb.Duration) (int64, error) {
				return in.Seconds * 2, nil
			},
		}).Register(ts.Registry)

		assert.NoErr(t, ts.Initialize(ctx))

		svcCS := config.Set("services/my-app")
		p1CS := config.Set("projects/p1")
		p2CS := config.Set("projects/p2")

		// 1. Seed service and project configs.
		ts.SetConfig(svcCS, "svc.cfg", durationpb.New(7*time.Second))
		ts.SetConfig(p1CS, "proj.cfg", durationpb.New(10*time.Second))
		ts.SetConfig(p2CS, "proj.cfg", durationpb.New(25*time.Second))
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		gotSvc, err := svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(7)))

		gotAll, err := proj.GetAllData(ctx, []string{"p1", "p2", "missing"})
		assert.NoErr(t, err)
		assert.That(t, gotAll, should.Match(map[string]int64{
			"p1": 20,
			"p2": 50,
		}))

		// 2. Update service config and delete p1.
		ts.SetConfig(svcCS, "svc.cfg", durationpb.New(9*time.Second))
		ts.SetConfig(p1CS, "proj.cfg", nil)
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(9)))

		gotP1, err := proj.GetData(ctx, "p1")
		assert.NoErr(t, err)
		assert.That(t, gotP1, should.Equal(int64(0)))

		// 3. Delete service config; returns MemoryDefault.
		ts.SetConfig(svcCS, "svc.cfg", nil)
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(-1)))
	})

	t.Run("ToMemory error preserves existing data and retries on next refresh", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		var fail atomic.Bool
		fail.Store(true)
		svc := (&cfgcache.ServicePipeline[*durationpb.Duration, int64]{
			ID:            "svc",
			Store:         s,
			Files:         []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "svc.cfg"}},
			MemoryDefault: func() int64 { return -1 },
			ToMemory: func(ctx context.Context, cs config.Set, in *durationpb.Duration) (int64, error) {
				if fail.Load() {
					return 0, errors.New("callback rejected config set")
				}
				return in.Seconds, nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		svcCS := config.Set("services/my-app")
		ts.SetConfig(svcCS, "svc.cfg", durationpb.New(7*time.Second))
		assert.NoErr(t, ts.IngestToStorage(ctx))

		// Initial load with failing ToMemory logs and leaves MemoryDefault in place.
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotSvc, err := svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(-1)))

		// Next refresh after clearing failure re-fetches and applies 7.
		fail.Store(false)
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(7)))

		// Update config while ToMemory is failing; RefreshInMemory logs and
		// preserves 7.
		fail.Store(true)
		ts.SetConfig(svcCS, "svc.cfg", durationpb.New(9*time.Second))
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(7)))

		// Next refresh after clearing failure re-fetches and applies 9.
		fail.Store(false)
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Equal(int64(9)))
	})

	t.Run("MemoryDefault is returned when config is absent or deleted, but not when ToMemory returns zero value", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		svc := (&cfgcache.PassthroughServicePipeline[*durationpb.Duration]{
			ID:            "svc",
			Store:         s,
			Files:         []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "svc.cfg"}},
			MemoryDefault: func() *durationpb.Duration { return durationpb.New(-1 * time.Second) },
		}).Register(ts.Registry)

		proj := (&cfgcache.ProjectPipeline[*durationpb.Duration, time.Duration]{
			ID:    "proj",
			Store: s,
			Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "proj.cfg"}},
			MemoryDefault: func(project string) time.Duration {
				return -88 * time.Hour
			},
			ToMemory: func(ctx context.Context, cs config.Set, in *durationpb.Duration) (time.Duration, error) {
				return in.AsDuration() * 2, nil
			},
		}).Register(ts.Registry)

		assert.NoErr(t, ts.Initialize(ctx))
		// No configs in storage, but we want to look anyway and record the fact
		// that we looked.
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		svcCS := config.Set("services/my-app")
		p1CS := config.Set("projects/p1")

		// 1. Before any configs exist in storage, both return MemoryDefault.
		gotSvc, err := svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Match(durationpb.New(-1*time.Second)))

		gotP1, err := proj.GetData(ctx, "p1")
		assert.NoErr(t, err)
		assert.That(t, gotP1, should.Equal(-88*time.Hour))

		// 2. When ToMemory returns the zero value for an existing config, GetData
		// returns that zero value rather than MemoryDefault, while unconfigured
		// projects still return MemoryDefault.
		ts.SetConfig(svcCS, "svc.cfg", durationpb.New(7*time.Second))
		ts.SetConfig(p1CS, "proj.cfg", durationpb.New(0*time.Second))
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		gotP1, err = proj.GetData(ctx, "p1")
		assert.NoErr(t, err)
		assert.That(t, gotP1, should.Equal(time.Duration(0)))

		gotAll, err := proj.GetAllData(ctx, []string{"p1", "p2"})
		assert.NoErr(t, err)
		assert.That(t, gotAll, should.Match(map[string]time.Duration{
			"p1": 0,
			"p2": -88 * time.Hour,
		}))

		// 3. Delete configs from storage; both return MemoryDefault again.
		ts.SetConfig(svcCS, "svc.cfg", nil)
		ts.SetConfig(p1CS, "proj.cfg", nil)
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc, should.Match(durationpb.New(-1*time.Second)))

		gotP1, err = proj.GetData(ctx, "p1")
		assert.NoErr(t, err)
		assert.That(t, gotP1, should.Equal(-88*time.Hour))
	})

	t.Run("version bounds out of range are skipped while loading and preserving compatible data", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		proj := (&cfgcache.PassthroughProjectPipeline[*durationpb.Duration]{
			ID:                 "proj",
			Store:              s,
			Files:              []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "cfg.cfg"}},
			TransformVersion:   2,
			ToMemoryMinVersion: 2,
			ToMemoryMaxVersion: 3,
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		assert.NoErr(t, s.Put(ctx, "proj", "projects/ok", &cfgcachepb.Meta{TransformVersion: 2, InputHash: "h2"}, durationpb.New(2*time.Second)))
		assert.NoErr(t, s.Put(ctx, "proj", "projects/new", &cfgcachepb.Meta{TransformVersion: 5, InputHash: "h5"}, durationpb.New(5*time.Second)))
		assert.NoErr(t, s.Put(ctx, "proj", "projects/old", &cfgcachepb.Meta{TransformVersion: 1, InputHash: "h1"}, durationpb.New(1*time.Second)))

		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotOk, err := proj.GetData(ctx, "ok")
		assert.NoErr(t, err)
		assert.That(t, gotOk.GetSeconds(), should.Equal(int64(2)))

		for _, skipped := range []string{"new", "old"} {
			got, err := proj.GetData(ctx, skipped)
			assert.NoErr(t, err)
			assert.That(t, got, should.Equal[*durationpb.Duration](nil))
		}

		// Updating "ok" to an out-of-range version preserves the previously loaded value.
		assert.NoErr(t, s.Put(ctx, "proj", "projects/ok", &cfgcachepb.Meta{TransformVersion: 5, InputHash: "h2-v5"}, durationpb.New(20*time.Second)))
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotOk, err = proj.GetData(ctx, "ok")
		assert.NoErr(t, err)
		assert.That(t, gotOk.GetSeconds(), should.Equal(int64(2)))
	})

	t.Run("storage error during refresh serves stale data within MaxMemoryStaleness and errors after expiry", func(t *testing.T) {
		t.Parallel()
		clockCtx, tc := testclock.UseTime(t.Context(), testclock.TestRecentTimeUTC)
		ctx, ts := cfgcache.TestingContext(clockCtx)
		ts.MaxMemoryStaleness = 30 * time.Second
		s := &mockStore[*durationpb.Duration]{}

		svc := (&cfgcache.PassthroughServicePipeline[*durationpb.Duration]{
			ID:    "svc",
			Store: s,
			Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "svc.cfg"}},
		}).Register(ts.Registry)
		proj := (&cfgcache.PassthroughProjectPipeline[*durationpb.Duration]{
			ID:    "proj",
			Store: s,
			Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "cfg.cfg"}},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		assert.NoErr(t, s.Put(ctx, "svc", "services/my-app", &cfgcachepb.Meta{InputHash: "hsvc"}, durationpb.New(9*time.Second)))
		assert.NoErr(t, s.Put(ctx, "proj", "projects/ok", &cfgcachepb.Meta{InputHash: "h2"}, durationpb.New(2*time.Second)))
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		// Inject LoadMeta error: EnsureFresh(ctx, true) fails immediately, while
		// GetData continues serving loaded data within MaxMemoryStaleness.
		s.setLoadMetaErr(errors.New("load metas boom"))
		tc.Add(ts.MaxMemoryStaleness)
		assert.ErrIsLike(t, ts.RefreshInMemory(ctx), "load metas boom")

		gotSvc, err := svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc.GetSeconds(), should.Equal(int64(9)))
		gotOk, err := proj.GetData(ctx, "ok")
		assert.NoErr(t, err)
		assert.That(t, gotOk.GetSeconds(), should.Equal(int64(2)))

		// Once MaxMemoryStaleness expires, GetData and GetAllData return the error.
		tc.Add(ts.MaxMemoryStaleness + time.Second)
		_, err = svc.GetData(ctx)
		assert.ErrIsLike(t, err, "load metas boom")
		_, err = proj.GetData(ctx, "ok")
		assert.ErrIsLike(t, err, "load metas boom")
		_, err = proj.GetAllData(ctx, []string{"ok"})
		assert.ErrIsLike(t, err, "load metas boom")

		// Clear LoadMeta error and refresh.
		s.setLoadMetaErr(nil)
		assert.NoErr(t, ts.RefreshInMemory(ctx))
		gotSvc, err = svc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotSvc.GetSeconds(), should.Equal(int64(9)))
	})

	t.Run("same config file path can be ingested into multiple pipelines", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		sharedFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "shared.cfg"}
		rawSvc := (&cfgcache.PassthroughServicePipeline[*durationpb.Duration]{
			ID:    "shared-raw",
			Store: s,
			Files: []cfgcache.File{sharedFile},
		}).Register(ts.Registry)
		scaledSvc := (&cfgcache.ServicePipeline[*durationpb.Duration, *durationpb.Duration]{
			ID:                 "shared-scaled",
			Store:              s,
			Files:              []cfgcache.File{sharedFile},
			TransformVersion:   1,
			ToMemoryMaxVersion: 1,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				msg := sharedFile.Get(snap)
				return &durationpb.Duration{Seconds: msg.Seconds * 10}, nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		ts.SetConfig("services/my-app", "shared.cfg", durationpb.New(5*time.Second))
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.NoErr(t, ts.RefreshInMemory(ctx))

		gotRaw, err := rawSvc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotRaw.GetSeconds(), should.Equal(int64(5)))

		gotScaled, err := scaledSvc.GetData(ctx)
		assert.NoErr(t, err)
		assert.That(t, gotScaled.GetSeconds(), should.Equal(int64(50)))
	})
}
