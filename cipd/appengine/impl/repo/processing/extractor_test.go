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
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExtractor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		var publishedRef *api.ObjectRef
		var canceled bool

		expectedUploadAlgo := api.HashAlgo_SHA256

		cas := testutil.MockCAS{
			BeginUploadImpl: func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				assert.Loosely(t, r.HashAlgo, should.Equal(expectedUploadAlgo))
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
			CancelUploadImpl: func(_ context.Context, r *api.CancelUploadRequest) (*api.UploadOperation, error) {
				assert.Loosely(t, r.UploadOperationId, should.Equal("op_id"))
				canceled = true
				return &api.UploadOperation{Status: api.UploadStatus_CANCELED}, nil
			},
		}

		extracted := bytes.Buffer{}
		uploader := &trackingWriter{w: &extracted}

		testFileBody := strings.Repeat("01234567", 8960)

		ex := Extractor{
			Reader: packageReader(map[string]string{
				"test/file": testFileBody,
			}),
			CAS:               &cas,
			PrimaryHash:       api.HashAlgo_SHA256,
			AlternativeHashes: []api.HashAlgo{api.HashAlgo_SHA1},
			Uploader:          func(ctx context.Context, size int64, uploadURL string) io.Writer { return uploader },
			BufferSize:        64 * 1024,
		}

		t.Run("Happy path, SHA256", func(t *ftt.Test) {
			res, err := ex.Run(ctx, "test/file")
			assert.Loosely(t, err, should.BeNil)

			// Check the return value.
			assert.Loosely(t, res.Path, should.Equal("test/file"))
			assert.Loosely(t, res.Ref, should.Resemble(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: hexDigest(api.HashAlgo_SHA256, testFileBody),
			}))
			assert.Loosely(t, res.Size, should.Equal(len(testFileBody)))
			assert.Loosely(t, res.Hashes, should.HaveLength(2))
			for _, h := range []api.HashAlgo{api.HashAlgo_SHA1, api.HashAlgo_SHA256} {
				assert.Loosely(t, common.HexDigest(res.Hashes[h]), should.Equal(hexDigest(h, testFileBody)))
			}

			// Check it actually uploaded the correct thing.
			assert.Loosely(t, extracted.String(), should.Equal(testFileBody))
			assert.Loosely(t, publishedRef, should.Resemble(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: hexDigest(api.HashAlgo_SHA256, testFileBody),
			}))

			// Check it was written in 64 Kb chunks, NOT 32 Kb as used by zip.Reader.
			assert.Loosely(t, uploader.calls, should.Resemble([]int{64 * 1024, 6 * 1024}))
		})

		t.Run("No such file in the package", func(t *ftt.Test) {
			_, err := ex.Run(ctx, "unknown")
			assert.Loosely(t, err, should.ErrLike(`failed to open the file for reading: no file "unknown" inside the package`))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("Internal error when initiating the upload", func(t *ftt.Test) {
			cas.BeginUploadImpl = func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, status.Errorf(codes.Internal, "boo")
			}
			_, err := ex.Run(ctx, "test/file")
			assert.Loosely(t, err, should.ErrLike(`failed to open a CAS upload: rpc error: code = Internal desc = boo`))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("Internal error when finalizing the upload", func(t *ftt.Test) {
			cas.FinishUploadImpl = func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				return nil, status.Errorf(codes.Internal, "boo")
			}
			_, err := ex.Run(ctx, "test/file")
			assert.Loosely(t, err, should.ErrLike(`failed to finalize the CAS upload: rpc error: code = Internal desc = boo`))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("Asked to restart the upload", func(t *ftt.Test) {
			uploader.err = &gs.RestartUploadError{Offset: 124}

			_, err := ex.Run(ctx, "test/file")
			assert.Loosely(t, err, should.ErrLike(`asked to restart the upload from faraway offset: the upload should be restarted from offset 124`))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			assert.Loosely(t, canceled, should.BeTrue)
		})
	})
}

func packageReader(data map[string]string) *PackageReader {
	buf := bytes.NewReader(testutil.MakeZip(data))
	size := int64(buf.Len())
	r, _ := NewPackageReader(buf, size)
	return r
}

type trackingWriter struct {
	w     io.Writer
	calls []int // sizes of each pushed chunk
	err   error // err to return from Write
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.calls = append(w.calls, len(p))
	return w.w.Write(p)
}
