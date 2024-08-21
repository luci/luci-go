// Copyright 2024 The LUCI Authors.
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

// Package cipd is a CIPD client used by the Swarming server.
package cipd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const (
	testPackageName = "testing"
	// Just some syntactically correct IID.
	testBadIID = "Nsi5WgbnGsQVQHanRd6PggSWkV8uvrJ1q7A5cJFCymMC"
)

func TestClient(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Build a test CIPD package to serve in the test.
	out := bytes.Buffer{}
	testPin, err := builder.BuildInstance(ctx, builder.Options{
		Output:      &out,
		PackageName: testPackageName,
		Input: []fs.File{
			fs.NewTestFile("file1", "12345", fs.TestFileOpts{}),
			fs.NewTestFile("a/b/c", "abc", fs.TestFileOpts{Executable: true}),
		},
	})
	assert.Loosely(t, err, should.BeNil)
	testPackage := out.Bytes()

	// A fake CIPD backend.
	mockedRepo := &mockedCIPDRepo{testPin: testPin}
	rpcSrv := prpctest.Server{}
	cipdpb.RegisterRepositoryServer(&rpcSrv, mockedRepo)
	rpcSrv.Start(ctx)
	defer rpcSrv.Close()
	cipdSrv := "http://" + rpcSrv.Host

	// A fake Google Storage backend.
	var (
		brokenPackage  atomic.Bool
		missingPackage atomic.Bool
	)
	storageSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if strings.TrimPrefix(req.URL.Path, "/") == testPackageName && !missingPackage.Load() {
			rw.WriteHeader(http.StatusOK)
			if _, err := rw.Write(testPackage); err != nil {
				panic(err)
			}
			if brokenPackage.Load() {
				if _, err := rw.Write([]byte("extra stuff")); err != nil {
					panic(err)
				}
			}
		} else {
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	defer storageSrv.Close()
	mockedRepo.storageURL = storageSrv.URL

	cipd := Client{}

	ftt.Run("ResolveVersion OK", t, func(t *ftt.Test) {
		iid, err := cipd.ResolveVersion(ctx, cipdSrv, testPackageName, "latest")
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, iid, should.Equal(testPin.InstanceID))
	})

	ftt.Run("ResolveVersion unknown", t, func(t *ftt.Test) {
		_, err := cipd.ResolveVersion(ctx, cipdSrv, testPackageName, "unknown")
		assert.That(t, status.Code(err), should.Equal(codes.NotFound))
	})

	ftt.Run("ResolveVersion lies", t, func(t *ftt.Test) {
		_, err := cipd.ResolveVersion(ctx, cipdSrv, testPackageName, testBadIID)
		assert.That(t, status.Code(err), should.Equal(codes.Internal))
	})

	ftt.Run("FetchInstance OK", t, func(t *ftt.Test) {
		pkg, err := cipd.FetchInstance(ctx, cipdSrv, testPackageName, testPin.InstanceID)
		assert.Loosely(t, err, should.BeNil)
		defer func() { _ = pkg.Close(ctx, false) }()
		var files []string
		for _, f := range pkg.Files() {
			files = append(files, f.Name())
		}
		assert.That(t, files, should.Match([]string{
			"file1",
			"a/b/c",
			".cipdpkg/manifest.json",
		}))
	})

	ftt.Run("FetchInstance unknown pkg", t, func(t *ftt.Test) {
		_, err := cipd.FetchInstance(ctx, cipdSrv, "unknown", testPin.InstanceID)
		assert.That(t, status.Code(err), should.Equal(codes.NotFound))
	})

	ftt.Run("FetchInstance bad hash", t, func(t *ftt.Test) {
		brokenPackage.Store(true)
		defer brokenPackage.Store(false)
		_, err := cipd.FetchInstance(ctx, cipdSrv, testPackageName, testPin.InstanceID)
		assert.That(t, status.Code(err), should.Equal(codes.Internal))
	})

	ftt.Run("FetchInstance unexpected storage response", t, func(t *ftt.Test) {
		missingPackage.Store(true)
		defer missingPackage.Store(false)
		_, err := cipd.FetchInstance(ctx, cipdSrv, testPackageName, testPin.InstanceID)
		assert.That(t, status.Code(err), should.Equal(codes.Internal))
	})
}

type mockedCIPDRepo struct {
	cipdpb.UnimplementedRepositoryServer

	testPin    common.Pin
	storageURL string
}

func (r *mockedCIPDRepo) ResolveVersion(ctx context.Context, req *cipdpb.ResolveVersionRequest) (*cipdpb.Instance, error) {
	if req.Package == r.testPin.PackageName &&
		(req.Version == r.testPin.InstanceID ||
			req.Version == "latest" ||
			req.Version == testBadIID) {
		return &cipdpb.Instance{
			Package:  r.testPin.PackageName,
			Instance: common.InstanceIDToObjectRef(r.testPin.InstanceID),
		}, nil
	}
	return nil, status.Errorf(codes.NotFound, "unknown package")
}

func (r *mockedCIPDRepo) GetInstanceURL(ctx context.Context, req *cipdpb.GetInstanceURLRequest) (*cipdpb.ObjectURL, error) {
	if req.Package == r.testPin.PackageName &&
		common.ObjectRefToInstanceID(req.Instance) == r.testPin.InstanceID {
		return &cipdpb.ObjectURL{
			SignedUrl: fmt.Sprintf("%s/%s", r.storageURL, testPackageName),
		}, nil
	}
	return nil, status.Errorf(codes.NotFound, "no such package")
}
