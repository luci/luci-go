// Copyright 2021 The LUCI Authors.
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

package processing

import (
	"bytes"
	"context"
	"io"
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestBootstrapPackageExtractor(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		var publishedRef *api.ObjectRef

		ctx := memory.Use(context.Background())

		testInstance := instance(ctx, "some/pkg", api.HashAlgo_SHA256)
		testBody := "12345678"
		testBodyDigest := hexDigest(api.HashAlgo_SHA256, testBody)

		cas := testutil.MockCAS{
			BeginUploadImpl: func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				assert.Loosely(t, r.HashAlgo, should.Equal(api.HashAlgo_SHA256))
				return &api.UploadOperation{
					OperationId: "op_id",
					UploadUrl:   "http://example.com/upload",
				}, nil
			},
			FinishUploadImpl: func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				assert.Loosely(t, r.UploadOperationId, should.Equal("op_id"))
				publishedRef = r.ForceHash
				return &api.UploadOperation{Status: api.UploadStatus_PUBLISHED}, nil
			},
		}

		extracted := bytes.Buffer{}

		ex := BootstrapPackageExtractor{
			CAS:      &cas,
			uploader: func(ctx context.Context, size int64, uploadURL string) io.Writer { return &extracted },
		}

		t.Run("Happy path", func(t *ftt.Test) {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				"some_binary.exe":       testBody,
				"some_binary.bat":       "zzz",
				".cipdpkg/ignored":      "zzz",
				".cipdpkg/also ignored": "zzz",
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.BeNil)

			assert.Loosely(t, res.Result.(BootstrapExtractorResult), should.Resemble(BootstrapExtractorResult{
				File:       "some_binary.exe",
				HashAlgo:   "SHA256",
				HashDigest: testBodyDigest,
				Size:       int64(len(testBody)),
			}))

			assert.Loosely(t, extracted.String(), should.Equal(testBody))
			assert.Loosely(t, publishedRef, should.Resemble(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: testBodyDigest,
			}))
		})

		t.Run("Many files", func(t *ftt.Test) {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				"some_binary.exe":    testBody,
				"another_binary.exe": testBody,
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.ErrLike("contains multiple files"))
		})

		t.Run("No files", func(t *ftt.Test) {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				".cipdpkg/ignored":      "zzz",
				".cipdpkg/also ignored": "zzz",
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.ErrLike("contains no files"))
		})

		t.Run("Not at root", func(t *ftt.Test) {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				"dir/some_binary.exe": testBody,
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.ErrLike("not at the package root"))
		})
	})
}
