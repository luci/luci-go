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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/retry/transient"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExtractor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("With mocks", t, func() {
		var publishedRef *api.ObjectRef
		var canceled bool

		expectedUploadAlgo := api.HashAlgo_SHA256

		cas := testutil.MockCAS{
			BeginUploadImpl: func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				So(r.HashAlgo, ShouldEqual, expectedUploadAlgo)
				return &api.UploadOperation{
					OperationId: "op_id",
					UploadUrl:   "http://example.com/upload",
				}, nil
			},
			FinishUploadImpl: func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				So(r.UploadOperationId, ShouldEqual, "op_id")
				publishedRef = r.ForceHash
				return &api.UploadOperation{Status: api.UploadStatus_PUBLISHED}, nil
			},
			CancelUploadImpl: func(_ context.Context, r *api.CancelUploadRequest) (*api.UploadOperation, error) {
				So(r.UploadOperationId, ShouldEqual, "op_id")
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

		Convey("Happy path, SHA256", func() {
			res, err := ex.Run(ctx, "test/file")
			So(err, ShouldBeNil)

			// Check the return value.
			So(res.Path, ShouldEqual, "test/file")
			So(res.Ref, ShouldResembleProto, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: hexDigest(api.HashAlgo_SHA256, testFileBody),
			})
			So(res.Size, ShouldEqual, len(testFileBody))
			So(res.Hashes, ShouldHaveLength, 2)
			for _, h := range []api.HashAlgo{api.HashAlgo_SHA1, api.HashAlgo_SHA256} {
				So(common.HexDigest(res.Hashes[h]), ShouldEqual, hexDigest(h, testFileBody))
			}

			// Check it actually uploaded the correct thing.
			So(extracted.String(), ShouldEqual, testFileBody)
			So(publishedRef, ShouldResembleProto, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: hexDigest(api.HashAlgo_SHA256, testFileBody),
			})

			// Check it was written in 64 Kb chunks, NOT 32 Kb as used by zip.Reader.
			So(uploader.calls, ShouldResemble, []int{64 * 1024, 6 * 1024})
		})

		Convey("No such file in the package", func() {
			_, err := ex.Run(ctx, "unknown")
			So(err, ShouldErrLike, `failed to open the file for reading: no file "unknown" inside the package`)
			So(transient.Tag.In(err), ShouldBeFalse)
		})

		Convey("Internal error when initiating the upload", func() {
			cas.BeginUploadImpl = func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.Internal, "boo")
			}
			_, err := ex.Run(ctx, "test/file")
			So(err, ShouldErrLike, `failed to open a CAS upload: rpc error: code = Internal desc = boo`)
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("Internal error when finalizing the upload", func() {
			cas.FinishUploadImpl = func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.Internal, "boo")
			}
			_, err := ex.Run(ctx, "test/file")
			So(err, ShouldErrLike, `failed to finalize the CAS upload: rpc error: code = Internal desc = boo`)
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("Asked to restart the upload", func() {
			uploader.err = &gs.RestartUploadError{Offset: 124}

			_, err := ex.Run(ctx, "test/file")
			So(err, ShouldErrLike, `asked to restart the upload from faraway offset: the upload should be restarted from offset 124`)
			So(transient.Tag.In(err), ShouldBeTrue)
			So(canceled, ShouldBeTrue)
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
