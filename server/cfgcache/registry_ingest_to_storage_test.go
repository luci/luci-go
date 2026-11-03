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

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/cfgcache/memstore"
)

// fakeCfgClient wraps a config.Interface to inject errors or override behavior.
type fakeCfgClient struct {
	config.Interface
	getConfigs        func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error)
	getProjectConfigs func(ctx context.Context, path string, metaOnly bool) ([]config.Config, error)
	getProjects       func(ctx context.Context) ([]config.Project, error)
}

func (f *fakeCfgClient) GetConfigs(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	if f.getConfigs != nil {
		return f.getConfigs(ctx, configSet, filter, metaOnly)
	}
	return f.Interface.GetConfigs(ctx, configSet, filter, metaOnly)
}

func (f *fakeCfgClient) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	if f.getProjectConfigs != nil {
		return f.getProjectConfigs(ctx, path, metaOnly)
	}
	return f.Interface.GetProjectConfigs(ctx, path, metaOnly)
}

func (f *fakeCfgClient) GetProjects(ctx context.Context) ([]config.Project, error) {
	if f.getProjects != nil {
		return f.getProjects(ctx)
	}
	return f.Interface.GetProjects(ctx)
}

func TestIngestToStorage(t *testing.T) {
	t.Parallel()

	t.Run("missing cfgclient in context returns error", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		(&cfgcache.ServicePipeline[*durationpb.Duration]{
			ID:    "svc",
			Store: s,
			Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "some.cfg"}},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))
		assert.ErrIsLike(t, ts.IngestToStorage(memory.Use(t.Context())), "LUCI Config client is not available in the context")
	})

	t.Run("missing service config in LUCI Config is ignored", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		var svcTransforms atomic.Int32
		svcFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}
		(&cfgcache.ServicePipeline[*durationpb.Duration]{
			ID:    "svc-settings",
			Store: s,
			Files: []cfgcache.File{svcFile},
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				svcTransforms.Add(1)
				return svcFile.Get(snap), nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.That(t, svcTransforms.Load(), should.Equal[int32](0))
	})

	t.Run("creates, skips unchanged, updates modified, and deletes removed configs", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		sd := &memstore.Store[*durationpb.Duration]{}
		st := &memstore.Store[*timestamppb.Timestamp]{}

		var svcTransforms, projTransforms atomic.Int32
		svcFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}
		projFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "${appid}.cfg"}
		(&cfgcache.ServicePipeline[*durationpb.Duration]{
			ID:    "svc-settings",
			Store: sd,
			Files: []cfgcache.File{svcFile},
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				svcTransforms.Add(1)
				msg := svcFile.Get(snap)
				return &durationpb.Duration{Seconds: msg.Seconds * 10}, nil
			},
		}).Register(ts.Registry)
		(&cfgcache.ProjectPipeline[*timestamppb.Timestamp]{
			ID:               "proj-cfg",
			Store:            st,
			Files:            []cfgcache.File{projFile},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*timestamppb.Timestamp, error) {
				projTransforms.Add(1)
				msg := projFile.Get(snap)
				return &timestamppb.Timestamp{Seconds: msg.Seconds}, nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		// 1. Initial creation.
		ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: 5})
		ts.SetConfig("projects/p1", "my-app.cfg", &durationpb.Duration{Seconds: 100})
		ts.SetConfig("projects/p2", "my-app.cfg", &durationpb.Duration{Seconds: 200})

		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.That(t, svcTransforms.Load(), should.Equal[int32](1))
		assert.That(t, projTransforms.Load(), should.Equal[int32](2))

		svcMetas, err := sd.LoadMeta(ctx, "svc-settings")
		assert.NoErr(t, err)
		assert.That(t, len(svcMetas), should.Equal(1))

		projMetas, err := st.LoadMeta(ctx, "proj-cfg")
		assert.NoErr(t, err)
		assert.That(t, len(projMetas), should.Equal(2))

		// 2. Re-running without changes skips Transform.
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.That(t, svcTransforms.Load(), should.Equal[int32](1))
		assert.That(t, projTransforms.Load(), should.Equal[int32](2))

		// 3. Modify p1 and delete p2; only p1 is re-transformed and p2 is removed.
		ts.SetConfig("projects/p1", "my-app.cfg", &durationpb.Duration{Seconds: 150})
		ts.SetConfig("projects/p2", "my-app.cfg", nil)

		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.That(t, svcTransforms.Load(), should.Equal[int32](1))
		assert.That(t, projTransforms.Load(), should.Equal[int32](3))

		projMetas, err = st.LoadMeta(ctx, "proj-cfg")
		assert.NoErr(t, err)
		assert.That(t, len(projMetas), should.Equal(1))
		assert.Loosely(t, projMetas, should.NotContainKey("projects/p2"))
	})

	t.Run("TransformVersion bump forces re-ingestion of unchanged config", func(t *testing.T) {
		t.Parallel()
		ctx, tsV1 := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		var transforms atomic.Int32
		projFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "proj.cfg"}
		(&cfgcache.ProjectPipeline[*durationpb.Duration]{
			ID:               "proj-cfg",
			Store:            s,
			Files:            []cfgcache.File{projFile},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				transforms.Add(1)
				return projFile.Get(snap), nil
			},
		}).Register(tsV1.Registry)
		assert.NoErr(t, tsV1.Initialize(ctx))

		p1CS := config.Set("projects/p1")
		tsV1.SetConfig(p1CS, "proj.cfg", &durationpb.Duration{Seconds: 100})
		assert.NoErr(t, tsV1.IngestToStorage(ctx))
		assert.That(t, transforms.Load(), should.Equal[int32](1))

		// New Registry with TransformVersion bumped to 2 against the same store/cfgclient.
		_, tsV2 := cfgcache.TestingContext(ctx)
		(&cfgcache.ProjectPipeline[*durationpb.Duration]{
			ID:               "proj-cfg",
			Store:            s,
			Files:            []cfgcache.File{projFile},
			TransformVersion: 2,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				transforms.Add(1)
				return projFile.Get(snap), nil
			},
		}).Register(tsV2.Registry)
		assert.NoErr(t, tsV2.Initialize(ctx))
		assert.NoErr(t, tsV2.IngestToStorage(ctx))
		assert.That(t, transforms.Load(), should.Equal[int32](2))

		metas, err := s.LoadMeta(ctx, "proj-cfg")
		assert.NoErr(t, err)
		assert.That(t, metas[p1CS].TransformVersion, should.Equal(2))
	})

	t.Run("validation warnings do not block ingestion", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		svcFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}
		(&cfgcache.ServicePipeline[*durationpb.Duration]{
			ID:    "svc",
			Store: s,
			Files: []cfgcache.File{svcFile},
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				c.Warningf("non-fatal warning")
				return svcFile.Get(snap), nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: 1})
		assert.NoErr(t, ts.IngestToStorage(ctx))

		metas, err := s.LoadMeta(ctx, "svc")
		assert.NoErr(t, err)
		assert.That(t, len(metas), should.Equal(1))
	})

	t.Run("multi-file project pipeline ingests combined files", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		fA := &cfgcache.SingleFile[*durationpb.Duration]{Path: "a.cfg", NewDefault: func() *durationpb.Duration { return &durationpb.Duration{Seconds: 1} }}
		fB := &cfgcache.SingleFile[*durationpb.Duration]{Path: "b.cfg", NewDefault: func() *durationpb.Duration { return &durationpb.Duration{Seconds: 2} }}
		(&cfgcache.ProjectPipeline[*durationpb.Duration]{
			ID:               "proj-ab",
			Store:            s,
			Files:            []cfgcache.File{fA, fB},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				return &durationpb.Duration{Seconds: fA.Get(snap).Seconds + fB.Get(snap).Seconds}, nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		p1CS := config.Set("projects/p1")
		p2CS := config.Set("projects/p2")
		ts.SetConfig(p1CS, "a.cfg", &durationpb.Duration{Seconds: 10})
		ts.SetConfig(p1CS, "b.cfg", &durationpb.Duration{Seconds: 20})
		ts.SetConfig(p2CS, "a.cfg", &durationpb.Duration{Seconds: 5})

		assert.NoErr(t, ts.IngestToStorage(ctx))

		abMetas, err := s.LoadMeta(ctx, "proj-ab")
		assert.NoErr(t, err)
		assert.That(t, len(abMetas), should.Equal(2))
		assert.Loosely(t, abMetas, should.ContainKey(p1CS))
		assert.Loosely(t, abMetas, should.ContainKey(p2CS))
	})

	t.Run("cfgcache.RegexFile project pipeline fetches matching files across all projects", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		rxF := &cfgcache.RegexFile[*durationpb.Duration]{PathRegex: `shards/.*\.cfg`}
		(&cfgcache.ProjectPipeline[*durationpb.Duration]{
			ID:               "proj-rx",
			Store:            s,
			Files:            []cfgcache.File{rxF},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				var sum int64
				for _, m := range rxF.Get(snap) {
					sum += m.Seconds
				}
				return &durationpb.Duration{Seconds: sum}, nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		ts.SetConfig("projects/p1", "shards/1.cfg", &durationpb.Duration{Seconds: 7})
		ts.SetConfig("projects/p2", "other.cfg", &durationpb.Duration{Seconds: 9})
		assert.NoErr(t, ts.IngestToStorage(ctx))

		rxMetas, err := s.LoadMeta(ctx, "proj-rx")
		assert.NoErr(t, err)
		assert.That(t, len(rxMetas), should.Equal(1))
		assert.Loosely(t, rxMetas, should.ContainKey(config.Set("projects/p1")))
	})

	t.Run("deduplicates RPCs across multiple service and project pipelines", func(t *testing.T) {
		t.Parallel()

		t.Run("multiple non-regex project pipelines share one GetProjects and per-project GetConfigs sweep", func(t *testing.T) {
			t.Parallel()
			ctx, ts := cfgcache.TestingContext(t.Context())
			s1 := &memstore.Store[*durationpb.Duration]{}
			s2 := &memstore.Store[*durationpb.Duration]{}

			fShared := &cfgcache.SingleFile[*durationpb.Duration]{Path: "shared.cfg"}
			fOnly2 := &cfgcache.SingleFile[*durationpb.Duration]{Path: "only2.cfg", NewDefault: func() *durationpb.Duration { return &durationpb.Duration{} }}
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:    "proj-1",
				Store: s1,
				Files: []cfgcache.File{fShared},
			}).Register(ts.Registry)
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:               "proj-2",
				Store:            s2,
				Files:            []cfgcache.File{fShared, fOnly2},
				TransformVersion: 1,
				Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
					return &durationpb.Duration{Seconds: fShared.Get(snap).Seconds + fOnly2.Get(snap).Seconds}, nil
				},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))

			ts.SetConfig("projects/p1", "shared.cfg", &durationpb.Duration{Seconds: 10})
			ts.SetConfig("projects/p1", "only2.cfg", &durationpb.Duration{Seconds: 5})

			var getProjectsCalls, getProjCfgsCalls, metaGetCfgsCalls, fullGetCfgsCalls atomic.Int32
			baseClient := cfgclient.Client(ctx)
			countingCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: baseClient,
				getProjects: func(ctx context.Context) ([]config.Project, error) {
					getProjectsCalls.Add(1)
					return baseClient.GetProjects(ctx)
				},
				getProjectConfigs: func(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
					getProjCfgsCalls.Add(1)
					return baseClient.GetProjectConfigs(ctx, path, metaOnly)
				},
				getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
					if metaOnly {
						metaGetCfgsCalls.Add(1)
					} else {
						fullGetCfgsCalls.Add(1)
					}
					return baseClient.GetConfigs(ctx, configSet, filter, metaOnly)
				},
			})

			assert.NoErr(t, ts.IngestToStorage(countingCtx))
			assert.That(t, getProjectsCalls.Load(), should.Equal[int32](1))
			assert.That(t, getProjCfgsCalls.Load(), should.Equal[int32](0))
			assert.That(t, metaGetCfgsCalls.Load(), should.Equal[int32](1))
			assert.That(t, fullGetCfgsCalls.Load(), should.Equal[int32](1))
		})

		t.Run("regex and single-file project pipelines share one GetProjects and per-project GetConfigs sweep", func(t *testing.T) {
			t.Parallel()
			ctx, ts := cfgcache.TestingContext(t.Context())
			s1 := &memstore.Store[*durationpb.Duration]{}
			s2 := &memstore.Store[*durationpb.Duration]{}

			rxF := &cfgcache.RegexFile[*durationpb.Duration]{PathRegex: `shards/.*\.cfg`}
			singleF := &cfgcache.SingleFile[*durationpb.Duration]{Path: "proj.cfg"}
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:               "proj-rx",
				Store:            s1,
				Files:            []cfgcache.File{rxF},
				TransformVersion: 1,
				Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
					var sum int64
					for _, m := range rxF.Get(snap) {
						sum += m.Seconds
					}
					return &durationpb.Duration{Seconds: sum}, nil
				},
			}).Register(ts.Registry)
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:    "proj-single",
				Store: s2,
				Files: []cfgcache.File{singleF},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))

			ts.SetConfig("projects/p1", "shards/1.cfg", &durationpb.Duration{Seconds: 7})
			ts.SetConfig("projects/p1", "proj.cfg", &durationpb.Duration{Seconds: 3})

			var getProjectsCalls, getProjCfgsCalls, metaGetCfgsCalls, fullGetCfgsCalls atomic.Int32
			baseClient := cfgclient.Client(ctx)
			countingCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: baseClient,
				getProjects: func(ctx context.Context) ([]config.Project, error) {
					getProjectsCalls.Add(1)
					return baseClient.GetProjects(ctx)
				},
				getProjectConfigs: func(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
					getProjCfgsCalls.Add(1)
					return baseClient.GetProjectConfigs(ctx, path, metaOnly)
				},
				getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
					if metaOnly {
						metaGetCfgsCalls.Add(1)
					} else {
						fullGetCfgsCalls.Add(1)
					}
					return baseClient.GetConfigs(ctx, configSet, filter, metaOnly)
				},
			})

			assert.NoErr(t, ts.IngestToStorage(countingCtx))
			assert.That(t, getProjectsCalls.Load(), should.Equal[int32](1))
			assert.That(t, getProjCfgsCalls.Load(), should.Equal[int32](0))
			assert.That(t, metaGetCfgsCalls.Load(), should.Equal[int32](1))
			assert.That(t, fullGetCfgsCalls.Load(), should.Equal[int32](1))
		})
	})

	t.Run("concurrent change where fetched config matches existing Meta skips Transform", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		var transforms int
		svcFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}
		(&cfgcache.ServicePipeline[*durationpb.Duration]{
			ID:    "svc",
			Store: s,
			Files: []cfgcache.File{svcFile},
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				transforms++
				return svcFile.Get(snap), nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: 10})
		assert.NoErr(t, ts.IngestToStorage(ctx))
		assert.That(t, transforms, should.Equal(1))

		baseClient := cfgclient.Client(ctx)
		overrideCtx := cfgclient.Use(ctx, &fakeCfgClient{
			Interface: baseClient,
			getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
				cfgs, err := baseClient.GetConfigs(ctx, configSet, filter, metaOnly)
				if err == nil && metaOnly {
					for k, v := range cfgs {
						v.ContentHash = "transient-hash"
						cfgs[k] = v
					}
				}
				return cfgs, err
			},
		})

		assert.NoErr(t, ts.IngestToStorage(overrideCtx))
		assert.That(t, transforms, should.Equal(1))
	})

	t.Run("concurrent deletion between fetchMeta and GetConfigs skips config set", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}

		var transforms int
		svcFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}
		(&cfgcache.ServicePipeline[*durationpb.Duration]{
			ID:    "svc",
			Store: s,
			Files: []cfgcache.File{svcFile},
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				transforms++
				return svcFile.Get(snap), nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: 10})
		baseClient := cfgclient.Client(ctx)
		vanishedCtx := cfgclient.Use(ctx, &fakeCfgClient{
			Interface: baseClient,
			getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
				if !metaOnly {
					return nil, nil
				}
				return baseClient.GetConfigs(ctx, configSet, filter, metaOnly)
			},
		})

		assert.NoErr(t, ts.IngestToStorage(vanishedCtx))
		assert.That(t, transforms, should.Equal(0))
	})

	t.Run("memory semaphore caps oversized snapshots and limits concurrency", func(t *testing.T) {
		t.Parallel()
		ctx, ts := cfgcache.TestingContext(t.Context())
		s := &memstore.Store[*durationpb.Duration]{}
		ts.MaxIngestMemoryMB = 64

		var active, maxActive atomic.Int32
		projFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "proj.cfg"}
		(&cfgcache.ProjectPipeline[*durationpb.Duration]{
			ID:               "proj",
			Store:            s,
			Files:            []cfgcache.File{projFile},
			TransformVersion: 1,
			Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
				cur := active.Add(1)
				for {
					prev := maxActive.Load()
					if cur <= prev || maxActive.CompareAndSwap(prev, cur) {
						break
					}
				}
				defer active.Add(-1)
				return projFile.Get(snap), nil
			},
		}).Register(ts.Registry)
		assert.NoErr(t, ts.Initialize(ctx))

		ts.SetConfig("projects/p1", "proj.cfg", &durationpb.Duration{Seconds: 1})
		ts.SetConfig("projects/p2", "proj.cfg", &durationpb.Duration{Seconds: 2})

		baseClient := cfgclient.Client(ctx)
		hugeMetaCtx := cfgclient.Use(ctx, &fakeCfgClient{
			Interface: baseClient,
			getProjectConfigs: func(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
				cfgs, err := baseClient.GetProjectConfigs(ctx, path, metaOnly)
				if err == nil {
					for i := range cfgs {
						cfgs[i].ContentSize = 100 << 20
					}
				}
				return cfgs, err
			},
		})

		assert.NoErr(t, ts.IngestToStorage(hugeMetaCtx))
		assert.That(t, maxActive.Load(), should.Equal[int32](1))
	})

	t.Run("error propagation", func(t *testing.T) {
		t.Parallel()

		t.Run("blocking validation error", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			svcFile := &cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}
			(&cfgcache.ServicePipeline[*durationpb.Duration]{
				ID:    "svc",
				Store: s,
				Files: []cfgcache.File{svcFile},
				Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
					c.Errorf("negative seconds")
					return svcFile.Get(snap), nil
				},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: -1})
			assert.ErrIsLike(t, ts.IngestToStorage(ctx), "negative seconds")
		})

		t.Run("service GetConfigs metaOnly=true error", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			(&cfgcache.ServicePipeline[*durationpb.Duration]{
				ID:    "svc",
				Store: s,
				Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			errCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: cfgclient.Client(ctx),
				getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
					return nil, errors.New("meta fetch boom")
				},
			})
			assert.ErrIsLike(t, ts.IngestToStorage(errCtx), "meta fetch boom")
		})

		t.Run("single-file project GetProjectConfigs error", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:    "proj",
				Store: s,
				Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "proj.cfg"}},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			errCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: cfgclient.Client(ctx),
				getProjectConfigs: func(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
					return nil, errors.New("project meta boom")
				},
			})
			assert.ErrIsLike(t, ts.IngestToStorage(errCtx), "project meta boom")
		})

		t.Run("cfgcache.RegexFile project GetProjects error", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:               "proj-rx",
				Store:            s,
				Files:            []cfgcache.File{&cfgcache.RegexFile[*durationpb.Duration]{PathRegex: `shards/.*\.cfg`}},
				TransformVersion: 1,
				Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
					return &durationpb.Duration{}, nil
				},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			errCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: cfgclient.Client(ctx),
				getProjects: func(ctx context.Context) ([]config.Project, error) {
					return nil, errors.New("get projects boom")
				},
			})
			assert.ErrIsLike(t, ts.IngestToStorage(errCtx), "get projects boom")
		})

		t.Run("cfgcache.RegexFile project GetConfigs metaOnly=true error", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			(&cfgcache.ProjectPipeline[*durationpb.Duration]{
				ID:               "proj-rx",
				Store:            s,
				Files:            []cfgcache.File{&cfgcache.RegexFile[*durationpb.Duration]{PathRegex: `shards/.*\.cfg`}},
				TransformVersion: 1,
				Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*durationpb.Duration, error) {
					return &durationpb.Duration{}, nil
				},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			ts.SetConfig("projects/p1", "shards/1.cfg", &durationpb.Duration{Seconds: 1})
			errCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: cfgclient.Client(ctx),
				getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
					return nil, errors.New("regex get configs boom")
				},
			})
			assert.ErrIsLike(t, ts.IngestToStorage(errCtx), "regex get configs boom")
		})

		t.Run("full fetch GetConfigs metaOnly=false error", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			(&cfgcache.ServicePipeline[*durationpb.Duration]{
				ID:    "svc",
				Store: s,
				Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: 1})
			baseClient := cfgclient.Client(ctx)
			errCtx := cfgclient.Use(ctx, &fakeCfgClient{
				Interface: baseClient,
				getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
					if !metaOnly {
						return nil, errors.New("full fetch boom")
					}
					return baseClient.GetConfigs(ctx, configSet, filter, metaOnly)
				},
			})
			assert.ErrIsLike(t, ts.IngestToStorage(errCtx), "full fetch boom")
		})

		t.Run("context canceled before semaphore acquire", func(t *testing.T) {
			ctx, ts := cfgcache.TestingContext(t.Context())
			s := &memstore.Store[*durationpb.Duration]{}
			(&cfgcache.ServicePipeline[*durationpb.Duration]{
				ID:    "svc",
				Store: s,
				Files: []cfgcache.File{&cfgcache.SingleFile[*durationpb.Duration]{Path: "settings.cfg"}},
			}).Register(ts.Registry)
			assert.NoErr(t, ts.Initialize(ctx))
			ts.SetConfig("services/my-app", "settings.cfg", &durationpb.Duration{Seconds: 1})
			baseClient := cfgclient.Client(ctx)
			cancelCtx, cancel := context.WithCancel(ctx)
			cancelFetchCtx := cfgclient.Use(cancelCtx, &fakeCfgClient{
				Interface: baseClient,
				getConfigs: func(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
					cfgs, err := baseClient.GetConfigs(ctx, configSet, filter, metaOnly)
					if metaOnly {
						cancel()
					}
					return cfgs, err
				},
			})
			assert.ErrIsLike(t, ts.IngestToStorage(cancelFetchCtx), context.Canceled.Error())
		})
	})
}
