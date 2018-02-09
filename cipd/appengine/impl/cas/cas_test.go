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
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas/upload"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetObjectURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var signErr error
	impl := storageImpl{
		getGS: func(context.Context) gs.GoogleStorage { return &testutil.NoopGoogleStorage{} },
		settings: func(context.Context) (*settings.Settings, error) {
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

type mockedBeginUploadGS struct {
	testutil.NoopGoogleStorage

	exists bool
}

func (m *mockedBeginUploadGS) Exists(c context.Context, path string) (bool, error) {
	return m.exists, nil
}

func (m *mockedBeginUploadGS) StartUpload(c context.Context, path string) (string, error) {
	return "http://upload-url.example.com/for/+" + path, nil
}

func TestBeginUpload(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		uploaderId := identity.Identity("user:uploader@example.com")
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)

		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, testTime)
		ctx = auth.WithState(ctx, &authtest.FakeState{Identity: uploaderId})

		gsMock := &mockedBeginUploadGS{}

		impl := storageImpl{
			getGS: func(context.Context) gs.GoogleStorage { return gsMock },
			settings: func(context.Context) (*settings.Settings, error) {
				return &settings.Settings{TempGSPath: "/bucket/tmp_path"}, nil
			},
		}

		Convey("Success (no Object)", func() {
			resp, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				HashAlgo: api.HashAlgo_SHA1,
			})
			So(err, ShouldBeNil)

			// ID can be decoded back.
			opID, err := upload.UnwrapOpID(ctx, resp.OperationId, uploaderId)
			So(err, ShouldBeNil)
			So(opID, ShouldEqual, 1)

			// Rest of the response looks OK too.
			resp.OperationId = ""
			So(resp, ShouldResemble, &api.UploadOperation{
				UploadUrl: "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				Status:    api.UploadStatus_UPLOADING,
			})

			// Created the entity.
			op := upload.Operation{ID: 1}
			So(datastore.Get(ctx, &op), ShouldBeNil)

			So(op.CreatedTS.Equal(testTime), ShouldBeTrue)
			op.CreatedTS = time.Time{}
			So(op.UpdatedTS.Equal(testTime), ShouldBeTrue)
			op.UpdatedTS = time.Time{}

			So(op, ShouldResemble, upload.Operation{
				ID:         1,
				Status:     api.UploadStatus_UPLOADING,
				TempGSPath: "/bucket/tmp_path/1454472306_1",
				UploadURL:  "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				HashAlgo:   api.HashAlgo_SHA1,
				CreatedBy:  uploaderId,
			})
		})

		Convey("Success (Object is not present in the store)", func() {
			resp, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				Object: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("a", 40),
				},
			})
			So(err, ShouldBeNil)

			resp.OperationId = ""
			So(resp, ShouldResemble, &api.UploadOperation{
				UploadUrl: "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				Status:    api.UploadStatus_UPLOADING,
			})

			op := upload.Operation{ID: 1}
			So(datastore.Get(ctx, &op), ShouldBeNil)
			So(op, ShouldResemble, upload.Operation{
				ID:         1,
				Status:     api.UploadStatus_UPLOADING,
				TempGSPath: "/bucket/tmp_path/1454472306_1",
				UploadURL:  "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				HashAlgo:   api.HashAlgo_SHA1,
				HexDigest:  strings.Repeat("a", 40),
				CreatedBy:  uploaderId,
				CreatedTS:  op.CreatedTS,
				UpdatedTS:  op.UpdatedTS,
			})
		})

		Convey("Object already exists", func() {
			gsMock.exists = true
			_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				Object: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("a", 40),
				},
			})
			So(grpc.Code(err), ShouldEqual, codes.AlreadyExists)
		})

		Convey("Bad object", func() {
			_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				Object: &api.ObjectRef{
					HashAlgo: 1234,
				},
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'object'")
		})

		Convey("Bad hash_algo", func() {
			_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				HashAlgo: 1234,
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'hash_algo'")
		})

		Convey("Mismatch in hash_algo", func() {
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
