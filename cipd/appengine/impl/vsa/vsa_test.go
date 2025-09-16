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

package vsa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/appengine/impl/vsa/api"
)

func TestVSAClient(t *testing.T) {
	t.Parallel()

	t.Run("VerifySoftwareArtifact", func(t *testing.T) {
		ctx, _, _ := testutil.TestingContext()

		makeInst := func(pkg, iid string) *model.Instance {
			i := &model.Instance{
				InstanceID: iid,
				Package:    model.PackageKey(ctx, pkg),
			}
			assert.NoErr(t, datastore.Put(ctx, i))
			return i
		}

		var resp *api.VerifySoftwareArtifactResponse
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if resp == nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("msg"))
				return
			}
			b, err := protojson.Marshal(resp)
			assert.NoErr(t, err)
			w.Write(b)
		}))
		defer server.Close()

		var logEntry *api.VerifySoftwareArtifactLogEntry
		clt := &client{
			client: server.Client(),
			bqlog: func(ctx context.Context, m proto.Message) {
				logEntry = m.(*api.VerifySoftwareArtifactLogEntry)
			},
		}

		t.Run("noop", func(t *testing.T) {
			assert.NoErr(t, clt.Init(ctx))
			vsa := clt.VerifySoftwareArtifact(ctx, makeInst("a/b", strings.Repeat("a", 40)), "{}")
			check.Loosely(t, vsa, should.BeEmpty)
			check.Loosely(t, logEntry, should.BeNil)
		})

		clt.resourcePrefix = "cipd_package://test"

		t.Run("prefix without host", func(t *testing.T) {
			check.ErrIsLike(t, clt.Init(ctx),
				"-slsa-resource-prefix must be set with -software-verifier-host")
		})

		clt.softwareVerifierHost = "https://bad^host"

		t.Run("bad prefix", func(t *testing.T) {
			check.ErrIsLike(t, clt.Init(ctx),
				"bad -slsa-resource-prefix")
		})

		clt.resourcePrefix = "cipd_package://test/"

		t.Run("bad host", func(t *testing.T) {
			check.ErrIsLike(t, clt.Init(ctx),
				"bad -software-verifier-host")
		})

		clt.softwareVerifierHost = server.URL

		t.Run("ok", func(t *testing.T) {
			resp = &api.VerifySoftwareArtifactResponse{
				Allowed:             true,
				VerificationSummary: "vsa content",
			}
			assert.NoErr(t, clt.Init(ctx))
			vsa := clt.VerifySoftwareArtifact(ctx, makeInst("a/b", strings.Repeat("a", 40)), "{}")
			check.That(t, vsa, should.Equal("vsa content"))
			check.That(t, logEntry, should.Match(&api.VerifySoftwareArtifactLogEntry{
				Package:     "a/b",
				Instance:    strings.Repeat("a", 40),
				ResourceUri: "cipd_package://test/a/b",
				Allowed:     true,
				Timestamp:   clock.Now(ctx).UnixMicro(),
			}))
		})

		t.Run("failed", func(t *testing.T) {
			resp = nil
			assert.NoErr(t, clt.Init(ctx))
			vsa := clt.VerifySoftwareArtifact(ctx, makeInst("a/b", strings.Repeat("a", 40)), "{}")
			check.Loosely(t, vsa, should.BeEmpty)
			check.That(t, logEntry, should.Match(&api.VerifySoftwareArtifactLogEntry{
				Package:      "a/b",
				Instance:     strings.Repeat("a", 40),
				ResourceUri:  "cipd_package://test/a/b",
				ErrorMessage: "VerifySoftwareArtifact: bad response status: api returns error: 500 Internal Server Error: msg",
				Timestamp:    clock.Now(ctx).UnixMicro(),
			}))
		})
	})

	t.Run("Status Cache", func(t *testing.T) {
		ctx, clk, _ := testutil.TestingContext()
		clt := &client{cache: make(mockCache)}

		inst := &model.Instance{
			InstanceID: strings.Repeat("a", 40),
			Package:    model.PackageKey(ctx, "a/b"),
		}

		s, err := clt.GetStatus(ctx, inst)
		check.That(t, s, should.Equal(CacheStatusUnknown))
		check.NoErr(t, err)

		err = clt.SetStatus(ctx, inst, CacheStatusPending)
		check.NoErr(t, err)

		s, err = clt.GetStatus(ctx, inst)
		check.That(t, s, should.Equal(CacheStatusPending))
		check.NoErr(t, err)

		clk.Add(time.Minute)
		s, err = clt.GetStatus(ctx, inst)
		check.That(t, s, should.Equal(CacheStatusUnknown))
		check.NoErr(t, err)

		err = clt.SetStatus(ctx, inst, CacheStatusCompleted)
		check.NoErr(t, err)

		s, err = clt.GetStatus(ctx, inst)
		check.That(t, s, should.Equal(CacheStatusCompleted))
		check.NoErr(t, err)

		clk.Add(time.Minute)
		s, err = clt.GetStatus(ctx, inst)
		check.That(t, s, should.Equal(CacheStatusCompleted))
		check.NoErr(t, err)

		clk.Add(time.Minute * 10)
		s, err = clt.GetStatus(ctx, inst)
		check.That(t, s, should.Equal(CacheStatusUnknown))
		check.NoErr(t, err)
	})
}

type mockCache map[string]cacheValue
type cacheValue struct {
	value    []byte
	deadline time.Time
}

func (c mockCache) Get(ctx context.Context, key string) ([]byte, error) {
	if clock.Now(ctx).After(c[key].deadline) {
		return nil, caching.ErrCacheMiss
	}
	return c[key].value, nil
}

func (c mockCache) Set(ctx context.Context, key string, value []byte, exp time.Duration) error {
	c[key] = cacheValue{
		value:    value,
		deadline: clock.Now(ctx).Add(exp),
	}
	return nil
}
