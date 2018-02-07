// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"strings"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/settings"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetObjectURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var signErr error
	impl := storageImpl{
		settings: func(c context.Context) (*settings.Settings, error) {
			return &settings.Settings{StorageGSPath: "/bucket/path"}, nil
		},
		getSignedURL: func(c context.Context, gsPath, filename string, signer signerFactory, gs gs.GoogleStorage) (string, error) {
			return "http//signed.example.com" + gsPath + "?f=" + filename, signErr
		},
	}

	Convey("OK", t, func() {
		resp, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
			DownloadFilename: "file.name",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &api.ObjectURL{
			SignedUrl: "http//signed.example.com/bucket/path/SHA1/" +
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa?f=file.name",
		})
	})

	Convey("Bad object ref", t, func() {
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: "zzz",
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad 'object' field")
	})

	Convey("Bad filename", t, func() {
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
			DownloadFilename: "abc\ndef",
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad 'download_filename' field")
	})

	Convey("No such file", t, func() {
		signErr = errors.Reason("blah").Tag(grpcutil.NotFoundTag).Err()
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.NotFound)
		So(err, ShouldErrLike, "blah")
	})

	Convey("Internal error", t, func() {
		signErr = errors.Reason("internal").Err()
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.Unknown)
		So(err, ShouldErrLike, "internal")
	})
}

func TestBeginUpload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	impl := storageImpl{}

	Convey("Bad object", t, func() {
		_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
			Object: &api.ObjectRef{
				HashAlgo: 1234,
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad 'object'")
	})

	Convey("Bad hash_algo", t, func() {
		_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
			HashAlgo: 1234,
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad 'hash_algo'")
	})

	Convey("Mismatch in hash_algo", t, func() {
		_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("a", 40),
			},
			HashAlgo: 2,
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "'hash_algo' and 'object.hash_algo' do not match")
	})
}

func TestFinishUpload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	impl := storageImpl{}

	Convey("Bad force_hash", t, func() {
		_, err := impl.FinishUpload(ctx, &api.FinishUploadRequest{
			ForceHash: &api.ObjectRef{
				HashAlgo: 1234,
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad 'force_hash' field")
	})
}
