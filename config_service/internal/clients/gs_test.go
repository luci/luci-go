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

package clients

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func testSigningContext(t testing.TB) (context.Context, testclock.TestClock) {
	t.Helper()
	appID := "dev~some-app-id"
	svcAcct := "something@service-name.iam.gserviceaccount.com"
	ctx := memory.UseWithAppID(t.Context(), appID)
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
			AppID:              appID,
			ServiceAccountName: svcAcct,
		})
		return cfg
	})
	return ctx, tc
}

func TestCreateSignedURLs(t *testing.T) {
	t.Parallel()

	ctx, _ := testSigningContext(t)
	svcAcct := "something@service-name.iam.gserviceaccount.com"

	opts, err := getSignedURLOptions(ctx, http.MethodGet, 10, map[string]string{
		"Custom-Header":         "Value",
		"Another-Custom-Header": "Another-Value",
	})
	assert.NoErr(t, err)
	assert.That(t, opts.GoogleAccessID, should.Equal(svcAcct))
	assert.That(t, opts.Scheme, should.Equal(storage.SigningSchemeV4))
	assert.That(t, opts.Method, should.Equal(http.MethodGet))
	assert.That(t, opts.Expires, should.Match(clock.Now(ctx).Add(SignedURLExpireDur)))
	assert.That(t, opts.Headers, should.Match([]string{"Another-Custom-Header:Another-Value", "Custom-Header:Value"}))
	assert.That(t, opts.QueryParameters, should.Match(url.Values{
		"generation": []string{"10"},
	}))
}

type fakeGCSObject struct {
	generation     int64
	metageneration int64
	rawBytes       []byte
}

type fakeGCSServer struct {
	mu      sync.Mutex
	objects map[string]*fakeGCSObject // key: bucket + "/" + object
}

func newFakeGCS(t testing.TB, ctx context.Context) (*fakeGCSServer, GsClient) {
	t.Helper()
	f := &fakeGCSServer{
		objects: map[string]*fakeGCSObject{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.serveHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	sc, err := storage.NewClient(
		ctx,
		option.WithEndpoint(srv.URL+"/storage/v1/"),
		option.WithHTTPClient(srv.Client()),
		option.WithoutAuthentication(),
	)
	assert.NoErr(t, err)
	t.Cleanup(func() { _ = sc.Close() })

	return f, &prodClient{client: sc}
}

func (f *fakeGCSServer) putObject(bucket, object string, gen int64, rawBytes []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[bucket+"/"+object] = &fakeGCSObject{
		generation:     gen,
		metageneration: 1,
		rawBytes:       rawBytes,
	}
}

func (f *fakeGCSServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	escaped := r.URL.EscapedPath()

	if after, ok := strings.CutPrefix(escaped, "/storage/v1/b/"); ok {
		bucket, rest, found := strings.Cut(after, "/o/")
		if !found {
			http.NotFound(w, r)
			return
		}
		object, err := url.PathUnescape(rest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		key := bucket + "/" + object
		obj := f.objects[key]

		if qGen := r.URL.Query().Get("generation"); qGen != "" && obj != nil {
			wantGen, _ := strconv.ParseInt(qGen, 10, 64)
			if obj.generation != wantGen {
				obj = nil
			}
		}

		if r.Method == http.MethodGet && r.URL.Query().Get("alt") == "media" {
			if obj == nil {
				http.NotFound(w, r)
				return
			}
			f.writeMedia(w, obj)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if obj == nil {
				http.NotFound(w, r)
				return
			}
			f.writeAttrsJSON(w, bucket, object, obj)
		case http.MethodDelete:
			if obj == nil {
				http.NotFound(w, r)
				return
			}
			delete(f.objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
		}
		return
	}

	if after, ok := strings.CutPrefix(escaped, "/download/storage/v1/b/"); ok {
		bucket, rest, found := strings.Cut(after, "/o/")
		if !found {
			http.NotFound(w, r)
			return
		}
		object, err := url.PathUnescape(rest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		obj := f.objects[bucket+"/"+object]
		if qGen := r.URL.Query().Get("generation"); qGen != "" && obj != nil {
			wantGen, _ := strconv.ParseInt(qGen, 10, 64)
			if obj.generation != wantGen {
				obj = nil
			}
		}
		if obj == nil {
			http.NotFound(w, r)
			return
		}
		f.writeMedia(w, obj)
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/")
	bucket, object, found := strings.Cut(trimmed, "/")
	if !found {
		http.NotFound(w, r)
		return
	}
	obj := f.objects[bucket+"/"+object]
	if qGen := r.URL.Query().Get("generation"); qGen != "" && obj != nil {
		wantGen, _ := strconv.ParseInt(qGen, 10, 64)
		if obj.generation != wantGen {
			obj = nil
		}
	}
	if obj == nil {
		http.NotFound(w, r)
	} else {
		f.writeMedia(w, obj)
	}
}

func (f *fakeGCSServer) writeMedia(w http.ResponseWriter, obj *fakeGCSObject) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("X-Goog-Generation", strconv.FormatInt(obj.generation, 10))
	w.Header().Set("X-Goog-Metageneration", strconv.FormatInt(obj.metageneration, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(obj.rawBytes)
}

func (f *fakeGCSServer) writeAttrsJSON(w http.ResponseWriter, bucket, object string, obj *fakeGCSObject) {
	resp := map[string]any{
		"bucket":          bucket,
		"name":            object,
		"generation":      strconv.FormatInt(obj.generation, 10),
		"metageneration":  strconv.FormatInt(obj.metageneration, 10),
		"contentEncoding": "gzip",
		"size":            strconv.Itoa(len(obj.rawBytes)),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func TestServiceDataHandle(t *testing.T) {
	t.Parallel()

	ctx, _ := testSigningContext(t)
	fakeGCS, client := newFakeGCS(t, ctx)

	const bucket = "test-bucket"
	content := []byte("valid config content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	prodObj := fmt.Sprintf("configs/sha256/%s", digest)
	fakeGCS.putObject(bucket, prodObj, 55, gzipBytes(t, content))

	sdh := client.ServiceDataHandle(bucket, prodObj)

	// GetURL before Read does not pin generation.
	u1, err := sdh.GetURL(ctx)
	assert.NoErr(t, err)
	parsed1, err := url.Parse(u1)
	assert.NoErr(t, err)
	assert.That(t, parsed1.Query().Get("generation"), should.Equal(""))

	// Read gunzips and pins generation=55.
	got, err := sdh.Read(ctx)
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(content))

	u2, err := sdh.GetURL(ctx)
	assert.NoErr(t, err)
	parsed2, err := url.Parse(u2)
	assert.NoErr(t, err)
	assert.That(t, parsed2.Query().Get("generation"), should.Equal("55"))

	// Delete removes object and is idempotent on missing object.
	assert.NoErr(t, sdh.Delete(ctx))
	assert.NoErr(t, sdh.Delete(ctx))
}
