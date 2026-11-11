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
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/cfgcache/dsstore"
	"go.chromium.org/luci/server/cfgcache/memstore"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
)

type fakeCookieAuth struct {
	user *auth.User
}

func (f *fakeCookieAuth) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	return f.user, nil, nil
}

func (f *fakeCookieAuth) LoginURL(ctx context.Context, dest string) (string, error) {
	return "/auth/login?r=" + url.QueryEscape(dest), nil
}

func (f *fakeCookieAuth) LogoutURL(ctx context.Context, dest string) (string, error) {
	return "/auth/logout?r=" + url.QueryEscape(dest), nil
}

// writeOnlyMetaStore implements Store (including LoadMeta) without implementing GettableStore.
type writeOnlyMetaStore struct {
	metas map[config.Set]*cfgcachepb.Meta
}

func (*writeOnlyMetaStore) ModuleDependencies() []module.Dependency { return nil }

func (s *writeOnlyMetaStore) LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]*cfgcachepb.Meta, error) {
	return s.metas, nil
}

func (s *writeOnlyMetaStore) Put(ctx context.Context, pipelineID string, cs config.Set, m *cfgcachepb.Meta, out *durationpb.Duration) error {
	if s.metas == nil {
		s.metas = make(map[config.Set]*cfgcachepb.Meta)
	}
	s.metas[cs] = m
	return nil
}

func (s *writeOnlyMetaStore) Delete(ctx context.Context, pipelineID string, sets []config.Set) error {
	for _, cs := range sets {
		delete(s.metas, cs)
	}
	return nil
}

var xsrfInputRe = regexp.MustCompile(`name="xsrf_token"\s+value="([^"]+)"`)

func TestStatusPageACLs(t *testing.T) {
	t.Parallel()

	const adminGroup = "admin-group"

	setupServer := func(t *testing.T, prod bool, user *auth.User, groups ...string) (context.Context, http.Handler) {
		t.Helper()
		ctx, ts := TestingContext(t.Context())
		ctx = secrets.Use(ctx, &testsecrets.Store{})
		db := authtest.NewFakeDB()
		if user != nil {
			for _, g := range groups {
				db.AddMocks(authtest.MockMembership(user.Identity, g))
			}
		}
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.DBProvider = func(context.Context) (authdb.DB, error) {
				return db, nil
			}
			return cfg
		})

		store := &memstore.Store[*durationpb.Duration]{}
		(&PassthroughServicePipeline[*durationpb.Duration]{
			ID:    "svc-cfg",
			Store: store,
			Files: []File{&SingleFile[*durationpb.Duration]{Path: "svc.cfg"}},
		}).Register(ts.Registry)
		assert.NoErr(t, store.Put(ctx, "svc-cfg", "services/my-app", &cfgcachepb.Meta{InputHash: "h1"}, &durationpb.Duration{Seconds: 5}))

		opts := &ModuleOptions{
			Registry:          ts.Registry,
			Vars:              ts.Vars,
			Rules:             &validation.RuleSet{Vars: ts.Vars},
			CronDispatcher:    &cron.Dispatcher{},
			DisableValidation: true,
			DisableCron:       true,
			DisableInMemory:   true,
			StatusAdminGroup:  adminGroup,
		}
		host := &fakeModuleHost{cookieAuth: fakeCookieAuth{
			user: user,
		}}
		mod := NewModule(opts)
		_, err := mod.Initialize(ctx, host, module.HostOptions{Prod: prod})
		assert.NoErr(t, err)
		return ctx, host.Routes()
	}

	t.Run("non-prod allows anonymous requests", func(t *testing.T) {
		t.Parallel()
		ctx, handler := setupServer(t, false, nil)
		req := httptest.NewRequest(http.MethodGet, StatusPagePath, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.That(t, rec.Code, should.Equal(http.StatusOK))
		assert.That(t, rec.Body.String(), should.ContainSubstring("svc-cfg"))

		cfgReq := httptest.NewRequest(http.MethodGet, StatusConfigPath+"?pipeline=svc-cfg&config_set=services/my-app", nil).WithContext(ctx)
		cfgRec := httptest.NewRecorder()
		handler.ServeHTTP(cfgRec, cfgReq)
		assert.That(t, cfgRec.Code, should.Equal(http.StatusOK))
		assert.That(t, strings.TrimSpace(cfgRec.Body.String()), should.Equal(`"5s"`))
	})

	t.Run("prod redirects anonymous user when LoginURL is available", func(t *testing.T) {
		t.Parallel()
		ctx, handler := setupServer(t, true, nil)
		req := httptest.NewRequest(http.MethodGet, StatusPagePath, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.That(t, rec.Code, should.Equal(http.StatusFound))
		assert.That(t, rec.Header().Get("Location"), should.Equal("/auth/login?r=%2Fadmin%2Fcfgcache"))
	})

	t.Run("prod returns 403 for authenticated user not in admin group", func(t *testing.T) {
		t.Parallel()
		user := &auth.User{Identity: "user:outsider@example.com", Email: "outsider@example.com"}
		ctx, handler := setupServer(t, true, user)
		req := httptest.NewRequest(http.MethodGet, StatusPagePath, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.That(t, rec.Code, should.Equal(http.StatusForbidden))
		assert.That(t, rec.Body.String(), should.ContainSubstring(adminGroup))

		cfgReq := httptest.NewRequest(http.MethodGet, StatusConfigPath+"?pipeline=svc-cfg&config_set=services/my-app", nil).WithContext(ctx)
		cfgRec := httptest.NewRecorder()
		handler.ServeHTTP(cfgRec, cfgReq)
		assert.That(t, cfgRec.Code, should.Equal(http.StatusForbidden))
	})

	t.Run("prod allows member of default admins group", func(t *testing.T) {
		t.Parallel()
		user := &auth.User{Identity: "user:admin@example.com", Email: "admin@example.com"}
		ctx, handler := setupServer(t, true, user, adminGroup)
		req := httptest.NewRequest(http.MethodGet, StatusPagePath, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.That(t, rec.Code, should.Equal(http.StatusOK))
		assert.That(t, rec.Body.String(), should.ContainSubstring("user:admin@example.com"))
	})

	t.Run("prod allows Superuser", func(t *testing.T) {
		t.Parallel()
		user := &auth.User{Identity: "user:super@example.com", Superuser: true}
		ctx, handler := setupServer(t, true, user)
		req := httptest.NewRequest(http.MethodGet, StatusPagePath, nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.That(t, rec.Code, should.Equal(http.StatusOK))
	})
}

func TestStatusPageRenderingIngestAndConfigJSONPB(t *testing.T) {
	t.Parallel()

	baseCtx := memory.Use(t.Context())
	datastore.GetTestable(baseCtx).Consistent(true)
	ctx, ts := TestingContext(baseCtx)
	ctx = secrets.Use(ctx, &testsecrets.Store{})
	db := authtest.NewFakeDB()
	ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return db, nil
		}
		return cfg
	})

	// Use dsstore.Store for 1-svc to verify proto.Equal handles newly deserialized
	// *cfgcachepb.Meta pointers across LoadMeta calls, and use *cfgcachepb.Meta as
	// the payload message to exercise camelCase vs snake_case JSONPB encoding.
	svcStore := dsstore.Store[*cfgcachepb.Meta]{}
	projStore := &memstore.Store[*durationpb.Duration]{}
	noMemStore := &memstore.Store[*durationpb.Duration]{}
	writeOnlyStore := &writeOnlyMetaStore{}

	svc := (&PassthroughServicePipeline[*cfgcachepb.Meta]{
		ID:    "1-svc",
		Store: svcStore,
		Files: []File{&SingleFile[*cfgcachepb.Meta]{Path: "${appid}.cfg"}},
	}).Register(ts.Registry)

	var failP3 atomic.Bool
	proj := (&ProjectPipeline[*durationpb.Duration, int64]{
		ID:                 "2-proj",
		Store:              projStore,
		Files:              []File{&SingleFile[*durationpb.Duration]{Path: "proj.cfg"}},
		TransformVersion:   2,
		ToMemoryMinVersion: 2,
		ToMemoryMaxVersion: 3,
		ToMemory: func(ctx context.Context, cs config.Set, in *durationpb.Duration) (int64, error) {
			if cs == "projects/p3" && failP3.Load() {
				return 0, errors.New("p3 callback boom")
			}
			return in.Seconds, nil
		},
	}).Register(ts.Registry)

	(&ServicePipeline[*durationpb.Duration, NoMemoryRefresh]{
		ID:    "3-store-only",
		Store: noMemStore,
		Files: []File{&SingleFile[*durationpb.Duration]{Path: "store_only.cfg"}},
	}).Register(ts.Registry)

	(&ServicePipeline[*durationpb.Duration, NoMemoryRefresh]{
		ID:    "4-write-only",
		Store: writeOnlyStore,
		Files: []File{&SingleFile[*durationpb.Duration]{Path: "write_only.cfg"}},
	}).Register(ts.Registry)

	opts := &ModuleOptions{
		Registry:          ts.Registry,
		Vars:              ts.Vars,
		Rules:             &validation.RuleSet{Vars: ts.Vars},
		CronDispatcher:    &cron.Dispatcher{},
		DisableValidation: true,
		DisableCron:       true,
		DisableInMemory:   true,
		StatusAdminGroup:  "admin-group",
	}
	host := &fakeModuleHost{}
	mod := NewModule(opts)
	_, err := mod.Initialize(ctx, host, module.HostOptions{Prod: false})
	assert.NoErr(t, err)
	handler := host.Routes()

	// Seed initial valid configs and run an initial refresh so acceptedState is populated.
	assert.NoErr(t, svcStore.Put(ctx, "1-svc", "services/my-app", &cfgcachepb.Meta{
		InputRevision:    "rev1234567890abcdef",
		InputHash:        "abcdef1234567890",
		InputSize:        120,
		TransformedSize:  48,
		TransformVersion: 0,
	}, &cfgcachepb.Meta{
		InputRevision:   "payload-rev",
		TransformedSize: 42,
	}))
	assert.NoErr(t, projStore.Put(ctx, "2-proj", "projects/p1", &cfgcachepb.Meta{
		InputRevision:    "p1rev",
		InputHash:        "p1hash",
		InputSize:        10,
		TransformedSize:  4,
		TransformVersion: 2,
	}, &durationpb.Duration{Seconds: 11}))
	assert.NoErr(t, projStore.Put(ctx, "2-proj", "projects/p3", &cfgcachepb.Meta{
		InputRevision:    "p3rev-old",
		InputHash:        "p3old",
		InputSize:        10,
		TransformedSize:  4,
		TransformVersion: 2,
	}, &durationpb.Duration{Seconds: 30}))
	assert.NoErr(t, noMemStore.Put(ctx, "3-store-only", "services/my-app", &cfgcachepb.Meta{
		InputRevision:    "storeonly-rev",
		InputHash:        "storeonlyhash",
		InputSize:        8,
		TransformedSize:  4,
		TransformVersion: 0,
	}, &durationpb.Duration{Seconds: 5}))
	assert.NoErr(t, writeOnlyStore.Put(ctx, "4-write-only", "services/my-app", &cfgcachepb.Meta{
		InputRevision:    "writeonly-rev",
		InputHash:        "writeonlyhash",
		InputSize:        8,
		TransformedSize:  4,
		TransformVersion: 0,
	}, &durationpb.Duration{Seconds: 9}))
	assert.NoErr(t, ts.RefreshInMemory(ctx))

	// Inject a version mismatch on projects/p2 and a ToMemory callback failure on projects/p3.
	failP3.Store(true)
	assert.NoErr(t, projStore.Put(ctx, "2-proj", "projects/p2", &cfgcachepb.Meta{
		InputRevision:    "p2rev-bad",
		InputHash:        "p2badver",
		InputSize:        10,
		TransformedSize:  4,
		TransformVersion: 5,
	}, &durationpb.Duration{Seconds: 22}))
	assert.NoErr(t, projStore.Put(ctx, "2-proj", "projects/p3", &cfgcachepb.Meta{
		InputRevision:    "p3rev-new",
		InputHash:        "p3new",
		InputSize:        10,
		TransformedSize:  4,
		TransformVersion: 2,
	}, &durationpb.Duration{Seconds: 33}))
	assert.NoErr(t, ts.RefreshInMemory(ctx))

	// 1. GET /admin/cfgcache runs the in-memory sync and renders status for all 4 pipelines.
	req := httptest.NewRequest(http.MethodGet, StatusPagePath, nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.That(t, rec.Code, should.Equal(http.StatusOK))
	body := rec.Body.String()

	assert.That(t, body, should.ContainSubstring("1-svc"))
	assert.That(t, body, should.ContainSubstring("rev123456789"))
	assert.That(t, body, should.ContainSubstring(`title="rev1234567890abcdef"`))
	assert.That(t, body, should.ContainSubstring("abcdef123456"))
	assert.That(t, body, should.ContainSubstring("48 B"))
	assert.That(t, body, should.ContainSubstring("2-proj"))
	assert.That(t, body, should.ContainSubstring("VERSION_MISMATCH"))
	assert.That(t, body, should.ContainSubstring("transform version is 5 &gt; max(3)"))
	assert.That(t, body, should.ContainSubstring("TO_MEMORY_ERROR"))
	assert.That(t, body, should.ContainSubstring("p3 callback boom"))
	assert.That(t, body, should.ContainSubstring("p3rev-new"))
	assert.That(t, body, should.ContainSubstring("p3rev-old"))
	assert.That(t, body, should.ContainSubstring("3-store-only"))
	assert.That(t, body, should.ContainSubstring("STORE_ONLY"))
	assert.That(t, body, should.ContainSubstring("NoMemoryRefresh"))
	assert.That(t, body, should.ContainSubstring("4-write-only"))
	assert.That(t, body, should.ContainSubstring("write-only (not GettableStore)"))
	assert.That(t, body, should.ContainSubstring(`/admin/cfgcache/config?config_set=services%2Fmy-app&amp;pipeline=1-svc`))

	// 2. GET /admin/cfgcache/config returns JSONPB (both lowerCamelCase and snake_case) and validates inputs.
	getConfig := func(rawQuery string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, StatusConfigPath+"?"+rawQuery, nil).WithContext(ctx)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	// Default lowerCamelCase JSONPB encoding.
	cfgRec := getConfig("pipeline=1-svc&config_set=services/my-app")
	assert.That(t, cfgRec.Code, should.Equal(http.StatusOK))
	assert.That(t, cfgRec.Header().Get("Content-Type"), should.Equal("application/json; charset=utf-8"))
	assert.That(t, cfgRec.Header().Get("X-Cfgcache-Proto-Type"), should.Equal("luci.server.cfgcache.Meta"))
	assert.That(t, cfgRec.Header().Get("X-Cfgcache-Input-Revision"), should.Equal("rev1234567890abcdef"))
	assert.That(t, cfgRec.Header().Get("X-Cfgcache-Transformed-Size"), should.Equal("48"))
	assert.That(t, cfgRec.Body.String(), should.ContainSubstring(`"inputRevision": "payload-rev"`))
	assert.That(t, cfgRec.Body.String(), should.ContainSubstring(`"transformedSize": "42"`))

	// Proto field names (snake_case) via proto_names=1.
	snakeRec := getConfig("pipeline=1-svc&config_set=services/my-app&proto_names=1")
	assert.That(t, snakeRec.Code, should.Equal(http.StatusOK))
	assert.That(t, snakeRec.Body.String(), should.ContainSubstring(`"input_revision": "payload-rev"`))
	assert.That(t, snakeRec.Body.String(), should.ContainSubstring(`"transformed_size": "42"`))

	// Store-only pipeline backed by GettableStore also serves JSONPB.
	storeOnlyRec := getConfig("pipeline=3-store-only&config_set=services/my-app")
	assert.That(t, storeOnlyRec.Code, should.Equal(http.StatusOK))
	assert.That(t, storeOnlyRec.Header().Get("X-Cfgcache-Proto-Type"), should.Equal("google.protobuf.Duration"))
	assert.That(t, strings.TrimSpace(storeOnlyRec.Body.String()), should.Equal(`"5s"`))

	// Error cases on StatusConfigPath.
	assert.That(t, getConfig("pipeline=1-svc").Code, should.Equal(http.StatusBadRequest))
	assert.That(t, getConfig("pipeline=unknown&config_set=services/my-app").Code, should.Equal(http.StatusNotFound))
	assert.That(t, getConfig("pipeline=2-proj&config_set=projects/missing").Code, should.Equal(http.StatusNotFound))
	assert.That(t, getConfig("pipeline=4-write-only&config_set=services/my-app").Code, should.Equal(http.StatusBadRequest))

	// Extract XSRF token from the rendered form.
	m := xsrfInputRe.FindStringSubmatch(body)
	assert.That(t, len(m), should.Equal(2))
	xsrfTok := m[1]

	// 3. POST /admin/cfgcache/ingest without XSRF token fails with 403.
	postNoToken := httptest.NewRequest(http.MethodPost, StatusIngestPath, nil).WithContext(ctx)
	recNoToken := httptest.NewRecorder()
	handler.ServeHTTP(recNoToken, postNoToken)
	assert.That(t, recNoToken.Code, should.Equal(http.StatusForbidden))

	// 4. Seed LUCI Config in TestingState and trigger POST /admin/cfgcache/ingest with valid XSRF token.
	failP3.Store(false)
	ts.SetConfig("services/my-app", "my-app.cfg", &cfgcachepb.Meta{InputRevision: "from-luci-cfg", TransformedSize: 77})
	ts.SetConfig("services/my-app", "store_only.cfg", &durationpb.Duration{Seconds: 88})
	ts.SetConfig("services/my-app", "write_only.cfg", &durationpb.Duration{Seconds: 66})
	ts.SetConfig("projects/p1", "proj.cfg", &durationpb.Duration{Seconds: 99})

	form := url.Values{"xsrf_token": {xsrfTok}}
	postReq := httptest.NewRequest(http.MethodPost, StatusIngestPath, strings.NewReader(form.Encode())).WithContext(ctx)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	assert.That(t, postRec.Code, should.Equal(http.StatusOK))
	postBody := postRec.Body.String()
	assert.That(t, postBody, should.ContainSubstring("Import succeeded"))
	assert.That(t, postBody, should.NotContainSubstring("VERSION_MISMATCH"))
	assert.That(t, postBody, should.NotContainSubstring("TO_MEMORY_ERROR"))

	// Verify the imported values were synced into memory and JSONPB endpoint reflects the new config.
	gotSvc, err := svc.GetData(ctx)
	assert.NoErr(t, err)
	assert.That(t, gotSvc.GetTransformedSize(), should.Equal(int64(77)))

	gotP1, err := proj.GetData(ctx, "p1")
	assert.NoErr(t, err)
	assert.That(t, gotP1, should.Equal(int64(99)))

	afterIngestRec := getConfig("pipeline=1-svc&config_set=services/my-app")
	assert.That(t, afterIngestRec.Code, should.Equal(http.StatusOK))
	assert.That(t, afterIngestRec.Body.String(), should.ContainSubstring(`"inputRevision": "from-luci-cfg"`))
	assert.That(t, afterIngestRec.Header().Get("X-Cfgcache-Input-Revision"), should.NotEqual(""))
}
