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
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/prpc"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	casgrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/caspb/grpcpb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	repogrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/repopb/grpcpb"
	"go.chromium.org/luci/cipd/client/cipd/proxyclient"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

const (
	fakeHost           = "fake-host.example.com"
	fakePackage        = "fake/pkg"
	fakeInstanceSHA256 = "fake-sha256"
	fakeSignedURL      = "fake-signed-url"
	fakeCASBody        = "fake-cas-body"
)

func TestServer(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no unix sockets")
	}

	ctx := context.Background()

	socket := filepath.Join(t.TempDir(), "sock")

	listener, err := net.Listen("unix", socket)
	assert.NoErr(t, err)

	pt, err := proxyclient.NewProxyTransport((&url.URL{
		Scheme: "unix",
		Path:   socket,
	}).String())
	assert.NoErr(t, err)
	defer func() { assert.NoErr(t, pt.Close()) }()

	httpC := &http.Client{Transport: pt.RoundTripper}
	repoC := repogrpcpb.NewRepositoryClient(&prpc.Client{
		C:    httpC,
		Host: fakeHost,
		Options: &prpc.Options{
			Insecure: true,
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   5 * time.Millisecond,
						Retries: 10,
					},
				}
			},
		},
	})

	repoS := &repoSrv{
		t:  t,
		ob: NewCASURLObfuscator(),
	}

	srv := &Server{
		Listener:         listener,
		Repository:       repoS,
		Storage:          &casgrpcpb.UnimplementedStorageServer{},
		CASURLObfuscator: repoS.ob,
		CAS: func(obj *proxypb.ProxiedCASObject, rw http.ResponseWriter, req *http.Request) {
			assert.That(t, obj, should.Match(&proxypb.ProxiedCASObject{
				SignedUrl: fakeSignedURL,
			}))
			assert.That(t, req.Method, should.Equal("GET"))
			_, err := rw.Write([]byte(fakeCASBody))
			assert.NoErr(t, err)
		},
	}

	go func() { assert.NoErr(t, srv.Serve(ctx)) }()
	defer func() { assert.NoErr(t, srv.Stop(ctx)) }()

	res, err := repoC.GetInstanceURL(ctx, &repopb.GetInstanceURLRequest{
		Package: fakePackage,
		Instance: &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: fakeInstanceSHA256,
		},
	})
	assert.NoErr(t, err)
	assert.That(t, res.SignedUrl, should.HavePrefix("http://cipd.local/obj/"))

	resp, err := httpC.Get(res.SignedUrl)
	assert.NoErr(t, err)
	body, err := io.ReadAll(resp.Body)
	assert.NoErr(t, err)
	assert.NoErr(t, resp.Body.Close())

	assert.That(t, string(body), should.Equal(fakeCASBody))
}

type repoSrv struct {
	repogrpcpb.UnimplementedRepositoryServer

	t  *testing.T
	ob *CASURLObfuscator
}

func (s *repoSrv) GetInstanceURL(ctx context.Context, req *repopb.GetInstanceURLRequest) (*caspb.ObjectURL, error) {
	assert.That(s.t, TargetHost(ctx), should.Equal(fakeHost))

	assert.That(s.t, req, should.Match(&repopb.GetInstanceURLRequest{
		Package: fakePackage,
		Instance: &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: fakeInstanceSHA256,
		},
	}))

	url, err := s.ob.Obfuscate(&proxypb.ProxiedCASObject{
		SignedUrl: fakeSignedURL,
	})
	assert.NoErr(s.t, err)

	return &caspb.ObjectURL{
		SignedUrl: url,
	}, nil
}
