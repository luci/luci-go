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

package cli

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd"
)

func TestCmdProxy_ReadOnlyCacheDir(t *testing.T) {
	t.Parallel()

	cmd := cmdProxy(Parameters{})
	f := cmd.CommandRun().(*proxyRun)

	t.Run("Flag registration", func(t *testing.T) {
		flag := f.Flags.Lookup("read-only-cache-dir")
		assert.That(t, flag != nil, should.BeTrue)
		assert.That(t, flag.Usage, should.ContainSubstring(cipd.EnvReadOnlyCacheDir))
	})

	t.Run("Flag parsing", func(t *testing.T) {
		c := cmdProxy(Parameters{}).CommandRun().(*proxyRun)
		err := c.Flags.Parse([]string{"-read-only-cache-dir", "/tmp/custom_cache"})
		assert.NoErr(t, err)
		assert.That(t, c.readOnlyCacheDir, should.Equal("/tmp/custom_cache"))
	})

	t.Run("Env var resolution - relative path error", func(t *testing.T) {
		ctx := environ.New([]string{
			cipd.EnvReadOnlyCacheDir + "=relative/path",
		}).SetInCtx(context.Background())

		_, err := runProxy(ctx, authcli.Flags{}, "/dev/null", "-", "", false)
		assert.That(t, err, should.ErrLike("not an absolute path"))
	})

	t.Run("Env var resolution - bad policy error implies cache dir was accepted", func(t *testing.T) {
		absPath, err := filepath.Abs("/tmp/valid_abs_cache")
		assert.NoErr(t, err)

		ctx := environ.New([]string{
			cipd.EnvReadOnlyCacheDir + "=" + absPath,
		}).SetInCtx(context.Background())

		// With a bad policy file path, runProxy will proceed past readOnlyCacheDir resolution
		// and fail on readProxyPolicy, proving readOnlyCacheDir was accepted.
		_, err = runProxy(ctx, authcli.Flags{}, "/dev/null", "/nonexistent/policy.textpb", "", false)
		assert.That(t, err, should.ErrLike("missing proxy policy file"))
	})
}

func TestCmdProxy_DisableNetwork(t *testing.T) {
	t.Parallel()

	cmd := cmdProxy(Parameters{})
	f := cmd.CommandRun().(*proxyRun)

	t.Run("Flag registration", func(t *testing.T) {
		flag := f.Flags.Lookup("disable-network")
		assert.That(t, flag != nil, should.BeTrue)
	})

	t.Run("Flag parsing", func(t *testing.T) {
		c := cmdProxy(Parameters{}).CommandRun().(*proxyRun)
		err := c.Flags.Parse([]string{"-disable-network"})
		assert.NoErr(t, err)
		assert.That(t, c.disableNetwork, should.BeTrue)
	})

	t.Run("disabledRoundTripper returns 404", func(t *testing.T) {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoErr(t, err)
		resp, err := disabledRoundTripper{}.RoundTrip(req)
		assert.NoErr(t, err)
		assert.That(t, resp.StatusCode, should.Equal(404))
	})

	t.Run("Env var resolution - CIPD_DISABLE_NETWORK accepted", func(t *testing.T) {
		for _, val := range []string{"1", "true"} {
			ctx := environ.New([]string{
				cipd.EnvCIPDDisableNetwork + "=" + val,
			}).SetInCtx(context.Background())

			// With a bad policy file path, runProxy will proceed past env resolution
			// and fail on readProxyPolicy, proving env was accepted.
			_, err := runProxy(ctx, authcli.Flags{}, "/dev/null", "/nonexistent/policy.textpb", "", false)
			assert.That(t, err, should.ErrLike("missing proxy policy file"))
		}
	})
}
