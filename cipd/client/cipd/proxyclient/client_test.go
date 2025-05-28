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

package proxyclient

import (
	"runtime"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNewProxyTransport(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no unix sockets")
	}

	// Note: tests for actual communication are in the proxyserver package.

	cases := []struct {
		url  string
		err  any
		addr string
	}{
		{`unix:///abs/path`, nil, "/abs/path"},
		{`unix:///with%20space`, nil, "/with space"},
		{`:`, "malformed URL", ""},
		{`http://127.0.0.1`, "only unix:// scheme", ""},
		{`unix://./abs`, "unexpected path", ""},
		{`unix://`, "unexpected path", ""},
	}
	for _, cs := range cases {
		pt, err := NewProxyTransport(cs.url)
		assert.That(t, err, should.ErrLike(cs.err))
		if cs.err == nil {
			assert.That(t, pt.Address, should.Equal(cs.addr))
			assert.NoErr(t, pt.Close())
		}
	}
}
