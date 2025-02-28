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

package proxyserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

func TestProxyCAS(t *testing.T) {
	t.Parallel()

	const testResponse = "hello"

	testSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.That(t, req.Method, should.Equal("GET"))
		assert.That(t, req.URL.Path, should.Equal("/ok"))
		assert.That(t, req.Header.Get("Test-Header"), should.Equal("Header val"))
		rw.Header().Set("Resp-Header", "Resp header val")
		_, err := rw.Write([]byte(testResponse))
		assert.NoErr(t, err)
	}))
	defer testSrv.Close()

	t.Run("Proxy OK", func(t *testing.T) {
		policy := &proxypb.Policy{
			GetInstanceUrl: &proxypb.Policy_GetInstanceURLPolicy{},
		}

		var lastOp *CASOp
		proxy := NewProxyCAS(policy, func(ctx context.Context, op *CASOp) { lastOp = op })

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://doesntmatter.example.com/also-ignored", nil)
		req.Header.Set("Test-Header", "Header val")
		proxy(&proxypb.ProxiedCASObject{SignedUrl: testSrv.URL + "/ok"}, rr, req)

		assert.That(t, rr.Result().StatusCode, should.Equal(http.StatusOK))
		body, _ := io.ReadAll(rr.Result().Body)
		assert.That(t, string(body), should.Equal(testResponse))
		assert.That(t, rr.Result().Header.Get("Resp-Header"), should.Equal("Resp header val"))

		assert.That(t, lastOp.Downloaded, should.Equal(uint64(len(testResponse))))
	})

	t.Run("Proxy PUT", func(t *testing.T) {
		policy := &proxypb.Policy{
			GetInstanceUrl: &proxypb.Policy_GetInstanceURLPolicy{},
		}
		proxy := NewProxyCAS(policy, nil)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "http://doesntmatter.example.com/also-ignored", nil)
		proxy(&proxypb.ProxiedCASObject{SignedUrl: testSrv.URL + "/ok"}, rr, req)

		assert.That(t, rr.Result().StatusCode, should.Equal(http.StatusForbidden))
		body, _ := io.ReadAll(rr.Result().Body)
		assert.That(t, string(body), should.Equal("Forbidden by the CIPD proxy\n"))
	})

	t.Run("Proxy denied", func(t *testing.T) {
		policy := &proxypb.Policy{}
		proxy := NewProxyCAS(policy, nil)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://doesntmatter.example.com/also-ignored", nil)
		proxy(&proxypb.ProxiedCASObject{SignedUrl: testSrv.URL + "/ok"}, rr, req)

		assert.That(t, rr.Result().StatusCode, should.Equal(http.StatusForbidden))
		body, _ := io.ReadAll(rr.Result().Body)
		assert.That(t, string(body), should.Equal("Forbidden by the CIPD proxy\n"))
	})
}
