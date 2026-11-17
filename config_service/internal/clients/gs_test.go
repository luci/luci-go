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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC.Truncate(time.Second))
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
	customTime     time.Time
	metadata       map[string]string
	rawBytes       []byte
}

type fakeGCSServer struct {
	mu      sync.Mutex
	objects map[string]*fakeGCSObject // key: bucket + "/" + object

	downloads int
	patches   int
	deletes   int
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

func (f *fakeGCSServer) putObject(bucket, object string, gen int64, rawBytes []byte, customTime time.Time, md map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var mdCopy map[string]string
	if md != nil {
		mdCopy = make(map[string]string, len(md))
		for k, v := range md {
			mdCopy[k] = v
		}
	}
	f.objects[bucket+"/"+object] = &fakeGCSObject{
		generation:     gen,
		metageneration: 1,
		customTime:     customTime,
		metadata:       mdCopy,
		rawBytes:       rawBytes,
	}
}

func (f *fakeGCSServer) getObject(bucket, object string) *fakeGCSObject {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.objects[bucket+"/"+object]
}

func (f *fakeGCSServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	escaped := r.URL.EscapedPath()

	// JSON API metadata endpoints: /storage/v1/b/{bucket}/o/{escapedObject}
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
			f.downloads++
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
		case http.MethodPatch:
			if obj == nil {
				http.NotFound(w, r)
				return
			}
			if ifMeta := r.URL.Query().Get("ifMetagenerationMatch"); ifMeta != "" {
				wantMeta, _ := strconv.ParseInt(ifMeta, 10, 64)
				if obj.metageneration != wantMeta {
					http.Error(w, `{"error":{"code":412}}`, http.StatusPreconditionFailed)
					return
				}
			}
			var patch struct {
				CustomTime string            `json:"customTime"`
				Metadata   map[string]string `json:"metadata"`
			}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if patch.CustomTime != "" {
				t, err := time.Parse(time.RFC3339Nano, patch.CustomTime)
				if err == nil {
					obj.customTime = t
				}
			}
			if patch.Metadata != nil {
				if obj.metadata == nil {
					obj.metadata = map[string]string{}
				}
				for k, v := range patch.Metadata {
					obj.metadata[k] = v
				}
			}
			obj.metageneration++
			f.patches++
			f.writeAttrsJSON(w, bucket, object, obj)
		case http.MethodDelete:
			f.deletes++
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

	// Download endpoint: /download/storage/v1/b/{bucket}/o/{escapedObject}
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
		f.downloads++
		f.writeMedia(w, obj)
		return
	}

	// XML download path: /{bucket}/{object...}
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
		return
	}
	f.downloads++
	f.writeMedia(w, obj)
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
	if !obj.customTime.IsZero() {
		resp["customTime"] = obj.customTime.UTC().Format(time.RFC3339Nano)
	}
	if len(obj.metadata) > 0 {
		resp["metadata"] = obj.metadata
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func TestUserDataHandle(t *testing.T) {
	t.Parallel()

	const bucket = "test-bucket"
	content := []byte("valid config content")
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	object := "validation/users/u1/configs/sha256/" + digest

	t.Run("rejects malformed object path", func(t *testing.T) {
		t.Parallel()
		ctx, _ := testSigningContext(t)
		_, client := newFakeGCS(t, ctx)

		_, err := client.UserDataHandle(ctx, bucket, "validation/users/u1/configs/not-sha256/"+digest)
		assert.ErrIsLike(t, err, "object must end with .../sha256/<hexDigest>")

		_, err = client.UserDataHandle(ctx, bucket, "validation/users/u1/configs/sha256/xyz")
		assert.ErrIsLike(t, err, "object must end with .../sha256/<hexDigest>")
	})

	t.Run("missing object mints PutURL and rejects GetURL/Read", func(t *testing.T) {
		t.Parallel()
		ctx, _ := testSigningContext(t)
		_, client := newFakeGCS(t, ctx)

		hnd, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd.Exists(), should.BeFalse)

		putURL, err := hnd.PutURL(ctx)
		assert.NoErr(t, err)
		assert.That(t, putURL, should.ContainSubstring("X-Goog-SignedHeaders=content-encoding%3Bhost%3Bx-goog-content-length-range"))

		_, err = hnd.GetURL(ctx)
		assert.That(t, errors.Is(err, storage.ErrObjectNotExist), should.BeTrue)

		_, err = hnd.Read(ctx)
		assert.That(t, errors.Is(err, storage.ErrObjectNotExist), should.BeTrue)
	})

	t.Run("verifies unverified object, sets metadata, and pins generation", func(t *testing.T) {
		t.Parallel()
		ctx, tc := testSigningContext(t)
		fakeGCS, client := newFakeGCS(t, ctx)
		fakeGCS.putObject(bucket, object, 42, gzipBytes(t, content), time.Time{}, nil)

		hnd, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd.Exists(), should.BeTrue)
		assert.That(t, fakeGCS.downloads, should.Equal(1))
		assert.That(t, fakeGCS.patches, should.Equal(1))

		stored := fakeGCS.getObject(bucket, object)
		assert.That(t, stored.metadata[VerifiedSha256Key], should.Equal(digest))
		assert.That(t, stored.customTime.Equal(clock.Now(ctx).UTC()), should.BeTrue)

		// PutURL fails when Exists() is true.
		_, err = hnd.PutURL(ctx)
		assert.ErrIsLike(t, err, "object exists")

		// GetURL pins generation=42.
		getURL, err := hnd.GetURL(ctx)
		assert.NoErr(t, err)
		u, err := url.Parse(getURL)
		assert.NoErr(t, err)
		assert.That(t, u.Query().Get("generation"), should.Equal("42"))

		// Read decompresses pinned generation.
		got, err := hnd.Read(ctx)
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(content))

		// Second UserDataHandle within 10 minutes skips both download and PATCH.
		tc.Add(5 * time.Minute)
		downloadsBefore := fakeGCS.downloads
		patchesBefore := fakeGCS.patches
		hnd2, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd2.Exists(), should.BeTrue)
		assert.That(t, fakeGCS.downloads, should.Equal(downloadsBefore))
		assert.That(t, fakeGCS.patches, should.Equal(patchesBefore))

		// After 10 minutes, UserDataHandle skips download but touches CustomTime.
		tc.Add(6 * time.Minute)
		hnd3, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd3.Exists(), should.BeTrue)
		assert.That(t, fakeGCS.downloads, should.Equal(downloadsBefore))
		assert.That(t, fakeGCS.patches, should.Equal(patchesBefore+1))
	})

	t.Run("deletes object and returns missing handle on sha256 mismatch", func(t *testing.T) {
		t.Parallel()
		ctx, _ := testSigningContext(t)
		fakeGCS, client := newFakeGCS(t, ctx)
		fakeGCS.putObject(bucket, object, 77, gzipBytes(t, []byte("wrong content")), time.Time{}, nil)

		hnd, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd.Exists(), should.BeFalse)
		assert.That(t, fakeGCS.deletes, should.Equal(1))
		assert.That(t, fakeGCS.getObject(bucket, object) == nil, should.BeTrue)
	})

	t.Run("deletes object and returns missing handle on invalid gzip", func(t *testing.T) {
		t.Parallel()
		ctx, _ := testSigningContext(t)
		fakeGCS, client := newFakeGCS(t, ctx)
		fakeGCS.putObject(bucket, object, 88, []byte("not gzip data"), time.Time{}, nil)

		hnd, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd.Exists(), should.BeFalse)
		assert.That(t, fakeGCS.deletes, should.Equal(1))
		assert.That(t, fakeGCS.getObject(bucket, object) == nil, should.BeTrue)
	})

	t.Run("deletes object and returns missing handle on empty 0-byte object", func(t *testing.T) {
		t.Parallel()
		ctx, _ := testSigningContext(t)
		fakeGCS, client := newFakeGCS(t, ctx)
		fakeGCS.putObject(bucket, object, 89, nil, time.Time{}, nil)

		hnd, err := client.UserDataHandle(ctx, bucket, object)
		assert.NoErr(t, err)
		assert.That(t, hnd.Exists(), should.BeFalse)
		assert.That(t, fakeGCS.deletes, should.Equal(1))
		assert.That(t, fakeGCS.getObject(bucket, object) == nil, should.BeTrue)
	})
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
	fakeGCS.putObject(bucket, prodObj, 55, gzipBytes(t, content), time.Time{}, nil)

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
