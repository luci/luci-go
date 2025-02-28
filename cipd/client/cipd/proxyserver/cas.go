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
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

// CASOp records how many bytes passed through the CAS proxy.
type CASOp struct {
	Downloaded uint64 // number of bytes fetched from the remote CAS
	Uploaded   uint64 // number of bytes uploaded to the remote CAS
}

// Used to pass a parameter from http.Handler to ReverseProxy.Rewrite callback.
var ctxKey = "target *url.URL"

// NewProxyCAS creates a CAS proxy handler.
func NewProxyCAS(policy *proxypb.Policy, opLog func(ctx context.Context, op *CASOp)) CASHandler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			targetURL := pr.In.Context().Value(&ctxKey).(*url.URL)
			pr.Out.URL = targetURL
			pr.Out.Host = targetURL.Host
		},
	}

	return func(obj *proxypb.ProxiedCASObject, rw http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		if req.Method != "GET" || policy.GetInstanceUrl == nil {
			logging.Errorf(ctx, "Forbidden call to the CIPD CAS proxy: %s %s", req.Method, req.URL)
			http.Error(rw, "Forbidden by the CIPD proxy", http.StatusForbidden)
			return
		}

		parsed, err := url.Parse(obj.SignedUrl)
		if err != nil {
			logging.Errorf(ctx, "Failed to parse GCS URL: %s", err)
			http.Error(rw, "Broken target of the proxy call", http.StatusInternalServerError)
			return
		}

		if opLog != nil {
			counting := &countingResponseWriter{ResponseWriter: rw}
			rw = counting
			defer func() {
				opLog(ctx, &CASOp{
					Downloaded: counting.written.Load(),
				})
			}()
		}
		rp.ServeHTTP(rw, req.WithContext(context.WithValue(ctx, &ctxKey, parsed)))
	}
}

type countingResponseWriter struct {
	http.ResponseWriter

	written atomic.Uint64
}

func (wr *countingResponseWriter) Unwrap() http.ResponseWriter {
	return wr.ResponseWriter
}

func (wr *countingResponseWriter) Write(blob []byte) (int, error) {
	wrote, err := wr.ResponseWriter.Write(blob)
	wr.written.Add(uint64(wrote))
	return wrote, err
}
