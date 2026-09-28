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

package casimpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"

	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type failingAuthFlags struct {
	err error
}

func (af *failingAuthFlags) Register(_ *flag.FlagSet) {}
func (af *failingAuthFlags) Parse() error             { return nil }
func (af *failingAuthFlags) NewRBEClient(ctx context.Context, _ string, _ string, _ bool) (*client.Client, error) {
	return nil, af.err
}

func TestNewRBEClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run(`newRBEClient`, t, func(t *ftt.Test) {
		t.Run(`fails and writes dumpJSON`, func(t *ftt.Test) {
			dumpJSON := filepath.Join(t.TempDir(), "exit.json")
			flags := commonFlags{
				authFlags: &failingAuthFlags{err: errors.New("auth failed")},
			}
			cl, err := flags.newRBEClient(ctx, dumpJSON, true)
			assert.Loosely(t, cl, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)

			data, err := os.ReadFile(dumpJSON)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result string `json:"result"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("client_error"))
		})

		t.Run(`fails without dumpJSON`, func(t *ftt.Test) {
			flags := commonFlags{
				authFlags: &failingAuthFlags{err: errors.New("auth failed")},
			}
			cl, err := flags.newRBEClient(ctx, "", true)
			assert.Loosely(t, cl, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`success`, func(t *ftt.Test) {
			testEnv, cleanup := fakes.NewTestEnv(t)
			t.Cleanup(cleanup)

			flags := commonFlags{
				authFlags: &testAuthFlags{testEnv: testEnv},
			}
			cl, err := flags.newRBEClient(ctx, "", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl, should.NotBeNil)
			defer cl.Close()
		})
	})
}

func TestFetchAndFlattenTreeErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run(`fetchAndFlattenTree errors`, t, func(t *ftt.Test) {
		testEnv, cleanup := fakes.NewTestEnv(t)
		t.Cleanup(cleanup)

		cl, err := testEnv.Server.NewTestClient(ctx)
		assert.Loosely(t, err, should.BeNil)
		defer cl.Close()

		t.Run(`missing root dir proto`, func(t *ftt.Test) {
			dumpJSON := filepath.Join(t.TempDir(), "exit.json")
			nonExistentDigest := digest.Digest{
				Hash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				Size: 100,
			}
			outputs, err := fetchAndFlattenTree(ctx, cl, nonExistentDigest, dumpJSON)
			assert.Loosely(t, outputs, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)

			data, err := os.ReadFile(dumpJSON)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result       string `json:"result"`
				ErrorDetails struct {
					Digest string `json:"digest,omitempty"`
				} `json:"error_details,omitempty"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("digest_invalid"))
		})
	})
}

func TestInitCache(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run(`initCache`, t, func(t *ftt.Test) {
		t.Run(`empty cacheDir returns nil`, func(t *ftt.Test) {
			c, err := initCache("", cache.Policies{}, "")
			assert.Loosely(t, c, should.BeNil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`valid cacheDir returns cache that reads and writes`, func(t *ftt.Test) {
			dir := t.TempDir()
			policies := cache.Policies{
				MaxSize:  units.Size(1024 * 1024),
				MaxItems: 10,
			}
			c, err := initCache(dir, policies, "")
			assert.Loosely(t, c, should.NotBeNil)
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				assert.Loosely(t, c.Close(), should.BeNil)
			}()

			h := sha256.Sum256([]byte("test data"))
			digest := cache.HexDigest(hex.EncodeToString(h[:]))
			assert.Loosely(t, c.Add(ctx, digest, strings.NewReader("test data")), should.BeNil)
			assert.Loosely(t, c.Touch(digest), should.BeTrue)

			r, err := c.Read(digest)
			assert.Loosely(t, err, should.BeNil)
			defer r.Close()

			data, err := io.ReadAll(r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(data), should.Equal("test data"))
		})

		t.Run(`invalid cacheDir writes IOError to dumpJSON`, func(t *ftt.Test) {
			filePath := filepath.Join(t.TempDir(), "not_a_dir")
			assert.Loosely(t, os.WriteFile(filePath, []byte("data"), 0600), should.BeNil)
			dumpJSON := filepath.Join(t.TempDir(), "exit.json")

			c, err := initCache(filePath, cache.Policies{}, dumpJSON)
			assert.Loosely(t, c, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)

			data, err := os.ReadFile(dumpJSON)
			assert.Loosely(t, err, should.BeNil)

			var result struct {
				Result string `json:"result"`
			}
			assert.Loosely(t, json.Unmarshal(data, &result), should.BeNil)
			assert.Loosely(t, result.Result, should.Equal("io_error"))
		})

		t.Run(`invalid cacheDir fails without dumpJSON`, func(t *ftt.Test) {
			filePath := filepath.Join(t.TempDir(), "not_a_dir")
			assert.Loosely(t, os.WriteFile(filePath, []byte("data"), 0600), should.BeNil)

			c, err := initCache(filePath, cache.Policies{}, "")
			assert.Loosely(t, c, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
