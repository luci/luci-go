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

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/templates"

	configpb "go.chromium.org/luci/config_service/proto"
)

type fakeConfigsServer struct {
	configpb.UnimplementedConfigsServer
	cs *configpb.ConfigSet
}

func (f *fakeConfigsServer) GetConfigSet(ctx context.Context, req *configpb.GetConfigSetRequest) (*configpb.ConfigSet, error) {
	return f.cs, nil
}

func TestConfigSetPagePathQuery(t *testing.T) {
	t.Parallel()

	srv := &fakeConfigsServer{
		cs: &configpb.ConfigSet{
			Name:              "projects/my-proj",
			LastImportAttempt: &configpb.ConfigSet_Attempt{Success: true},
			Revision:          &configpb.ConfigSet_Revision{Timestamp: timestamppb.Now()},
			Configs: []*configpb.Config{
				{Path: "found.cfg", Url: "https://example.com/found.cfg"},
				{Path: "nourl.cfg", Url: ""},
			},
		},
	}

	call := func(t *testing.T, target string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
		ctx := auth.WithState(req.Context(), &authtest.FakeState{})
		ctx = secrets.Use(ctx, &testsecrets.Store{})
		ctx = context.WithValue(ctx, &pageStartTimeContextKey, time.Now().UTC())
		ctx = context.WithValue(ctx, &configsServerContextKey, configpb.ConfigsServer(srv))
		ctx = templates.Use(ctx, prepareTemplates(&server.Options{Prod: true}), &templates.Extra{Request: req})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		err := configSetPage(&router.Context{
			Writer:  rec,
			Request: req,
			Params:  httprouter.Params{{Key: "ConfigSet", Value: "/projects/my-proj"}},
		})
		assert.NoErr(t, err)
		return rec
	}

	t.Run("redirects when path has non-empty URL", func(t *testing.T) {
		t.Parallel()
		rec := call(t, "/config_set/projects/my-proj?path=found.cfg")
		assert.That(t, rec.Code, should.Equal(http.StatusFound))
		assert.That(t, rec.Header().Get("Location"), should.Equal("https://example.com/found.cfg"))
	})

	t.Run("renders page when path has empty URL", func(t *testing.T) {
		t.Parallel()
		rec := call(t, "/config_set/projects/my-proj?path=nourl.cfg")
		assert.That(t, rec.Code, should.Equal(http.StatusOK))
		assert.That(t, rec.Header().Get("Location"), should.Equal(""))
	})

	t.Run("renders page when path is not found", func(t *testing.T) {
		t.Parallel()
		rec := call(t, "/config_set/projects/my-proj?path=missing.cfg")
		assert.That(t, rec.Code, should.Equal(http.StatusOK))
		assert.That(t, rec.Header().Get("Location"), should.Equal(""))
	})
}
