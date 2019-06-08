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
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/appengine/tq/tqtesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/cas/upload"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetReader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	sha256 := strings.Repeat("a", 64)
	gsMock := &mockedGS{
		files: map[string]string{
			"/bucket/path/SHA256/" + sha256: "zzz body",
		},
	}

	impl := storageImpl{
		getGS: func(context.Context) gs.GoogleStorage { return gsMock },
		settings: func(context.Context) (*settings.Settings, error) {
			return &settings.Settings{StorageGSPath: "/bucket/path"}, nil
		},
	}

	Convey("OK", t, func() {
		r, err := impl.GetReader(ctx, &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: sha256,
		})
		So(err, ShouldBeNil)
		So(r, ShouldNotBeNil)

		body := make([]byte, 100)
		l, err := r.ReadAt(body, 0)
		So(err, ShouldEqual, io.EOF)
		body = body[:l]
		So(string(body), ShouldEqual, "zzz body")
	})

	Convey("Bad object ref", t, func() {
		_, err := impl.GetReader(ctx, &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: "zzz",
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad ref")
	})

	Convey("No such file", t, func() {
		_, err := impl.GetReader(ctx, &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("b", 64),
		})
		So(grpc.Code(err), ShouldEqual, codes.NotFound)
		So(err, ShouldErrLike, "can't read the object")
	})
}

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
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
			DownloadFilename: "file.name",
		})
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &api.ObjectURL{
			SignedUrl: "http//signed.example.com/bucket/path/SHA256/" +
				strings.Repeat("a", 64) + "?f=file.name",
		})
	})

	Convey("Bad object ref", t, func() {
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: "zzz",
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
		So(err, ShouldErrLike, "bad 'object' field")
	})

	Convey("Bad filename", t, func() {
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
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
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.NotFound)
		So(err, ShouldErrLike, "blah")
	})

	Convey("Internal error", t, func() {
		signErr = errors.Reason("internal").Err()
		_, err := impl.GetObjectURL(ctx, &api.GetObjectURLRequest{
			Object: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
		})
		So(grpc.Code(err), ShouldEqual, codes.Unknown)
		So(err, ShouldErrLike, "internal")
	})
}

type mockedGS struct {
	testutil.NoopGoogleStorage

	exists bool
	files  map[string]string

	publishErr  error
	publisCalls []publishCall

	deleteErr   error
	deleteCalls []string

	cancelUploadCalls []string
}

type publishCall struct {
	dst    string
	src    string
	srcGen int64
}

func (m *mockedGS) Exists(c context.Context, path string) (bool, error) {
	return m.exists, nil
}

func (m *mockedGS) StartUpload(c context.Context, path string) (string, error) {
	return "http://upload-url.example.com/for/+" + path, nil
}

func (m *mockedGS) CancelUpload(c context.Context, uploadURL string) error {
	m.cancelUploadCalls = append(m.cancelUploadCalls, uploadURL)
	return nil
}

func (m *mockedGS) Reader(c context.Context, path string, gen int64) (gs.Reader, error) {
	if body, ok := m.files[path]; ok {
		return mockedGSReader{Reader: strings.NewReader(body)}, nil
	}
	return nil, errors.Reason("file %q is missing", path).Tag(gs.StatusCodeTag(http.StatusNotFound)).Err()
}

func (m *mockedGS) Publish(c context.Context, dst, src string, srcGen int64) error {
	if m.publisCalls == nil {
		panic("didn't expect Publish calls")
	}
	m.publisCalls = append(m.publisCalls, publishCall{dst, src, srcGen})
	return m.publishErr
}

func (m *mockedGS) Delete(c context.Context, path string) error {
	if m.deleteCalls == nil {
		panic("didn't expect Delete calls")
	}
	m.deleteCalls = append(m.deleteCalls, path)
	return m.deleteErr
}

type mockedGSReader struct{ *strings.Reader }

func (m mockedGSReader) Generation() int64 { return 42 }

func TestBeginUpload(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		uploaderID := identity.Identity("user:uploader@example.com")

		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, testTime)
		ctx = auth.WithState(ctx, &authtest.FakeState{Identity: uploaderID})

		gsMock := &mockedGS{}

		impl := storageImpl{
			getGS: func(context.Context) gs.GoogleStorage { return gsMock },
			settings: func(context.Context) (*settings.Settings, error) {
				return &settings.Settings{TempGSPath: "/bucket/tmp_path"}, nil
			},
		}

		Convey("Success (no Object)", func() {
			resp, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				HashAlgo: api.HashAlgo_SHA256,
			})
			So(err, ShouldBeNil)

			// ID can be decoded back.
			opID, err := upload.UnwrapOpID(ctx, resp.OperationId, uploaderID)
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
				HashAlgo:   api.HashAlgo_SHA256,
				CreatedBy:  uploaderID,
			})
		})

		Convey("Success (Object is not present in the store)", func() {
			resp, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				Object: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
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
				HashAlgo:   api.HashAlgo_SHA256,
				HexDigest:  strings.Repeat("a", 64),
				CreatedBy:  uploaderID,
				CreatedTS:  op.CreatedTS,
				UpdatedTS:  op.UpdatedTS,
			})
		})

		Convey("Object already exists", func() {
			gsMock.exists = true
			_, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				Object: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
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
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
				},
				HashAlgo: 333, // something else
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "'hash_algo' and 'object.hash_algo' do not match")
		})
	})
}

func storageMocks() (context.Context, *mockedGS, tqtesting.Testable, *storageImpl) {
	testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
	uploaderID := identity.Identity("user:uploader@example.com")
	ctx := gaetesting.TestingContext()
	ctx, _ = testclock.UseTime(ctx, testTime)
	ctx = auth.WithState(ctx, &authtest.FakeState{Identity: uploaderID})

	gsMock := &mockedGS{
		files:       map[string]string{},
		publisCalls: []publishCall{},
		deleteCalls: []string{},
	}

	dispatcher := &tq.Dispatcher{BaseURL: "/internal/tq/"}

	impl := &storageImpl{
		tq:    dispatcher,
		getGS: func(context.Context) gs.GoogleStorage { return gsMock },
		settings: func(context.Context) (*settings.Settings, error) {
			return &settings.Settings{
				StorageGSPath: "/bucket/store",
				TempGSPath:    "/bucket/tmp_path",
			}, nil
		},
	}
	impl.registerTasks()

	tq := tqtesting.GetTestable(ctx, dispatcher)
	tq.CreateQueues()

	return ctx, gsMock, tq, impl
}

func TestFinishUpload(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		ctx, gsMock, tq, impl := storageMocks()

		Convey("With force hash", func() {
			// Initiate an upload to get operation ID.
			op, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				HashAlgo: api.HashAlgo_SHA256,
			})
			So(err, ShouldBeNil)
			So(op, ShouldResemble, &api.UploadOperation{
				OperationId: op.OperationId,
				Status:      api.UploadStatus_UPLOADING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			})

			// Pretend we've uploaded 5 bytes.
			gsMock.files["/bucket/tmp_path/1454472306_1"] = "12345"

			Convey("Success", func() {
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
					ForceHash: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId: op.OperationId,
					Status:      api.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})

				// Published the file, deleted the temporary one.
				So(gsMock.publisCalls, ShouldResemble, []publishCall{
					{
						dst:    "/bucket/store/SHA256/" + strings.Repeat("a", 64),
						src:    "/bucket/tmp_path/1454472306_1",
						srcGen: -1,
					},
				})
				So(gsMock.deleteCalls, ShouldResemble, []string{"/bucket/tmp_path/1454472306_1"})
			})

			Convey("Publish transient error", func() {
				gsMock.publishErr = errors.Reason("blarg").Tag(transient.Tag).Err()
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
					ForceHash: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})
				So(grpc.Code(err), ShouldEqual, codes.Internal)

				// Status untouched.
				entity := upload.Operation{ID: 1}
				So(datastore.Get(ctx, &entity), ShouldBeNil)
				So(entity.Status, ShouldEqual, api.UploadStatus_UPLOADING)
			})

			Convey("Publish fatal error", func() {
				gsMock.publishErr = errors.Reason("blarg").Err()
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
					ForceHash: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId:  op.OperationId,
					Status:       api.UploadStatus_ERRORED,
					UploadUrl:    "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					ErrorMessage: "Failed to publish the object - blarg",
				})
			})
		})

		Convey("Without force hash, unknown expected hash", func() {
			// Initiate an upload to get operation ID.
			op, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				HashAlgo: api.HashAlgo_SHA256,
			})
			So(err, ShouldBeNil)
			So(op, ShouldResemble, &api.UploadOperation{
				OperationId: op.OperationId,
				Status:      api.UploadStatus_UPLOADING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			})

			// Pretend we've uploaded 5 bytes.
			gsMock.files["/bucket/tmp_path/1454472306_1"] = "12345"

			// Kick off the verification.
			op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
				UploadOperationId: op.OperationId,
			})
			So(err, ShouldBeNil)
			So(op, ShouldResemble, &api.UploadOperation{
				OperationId: op.OperationId,
				Status:      api.UploadStatus_VERIFYING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			})

			// Posted the verification task.
			t := tq.GetScheduledTasks()
			So(len(t), ShouldEqual, 1)
			So(t[0].Payload, ShouldResemble, &tasks.VerifyUpload{UploadOperationId: 1})

			Convey("Retying FinishUpload does nothing", func() {
				// Retying the call does nothing.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op.Status, ShouldEqual, api.UploadStatus_VERIFYING)

				// Still only 1 task in the queue.
				t = tq.GetScheduledTasks()
				So(len(t), ShouldEqual, 1)
			})

			Convey("Successful verification", func() {
				// Execute the pending verification task.
				So(impl.verifyUploadTask(ctx, t[0].Payload.(*tasks.VerifyUpload)), ShouldBeNil)

				// Published the verified file, deleted the temporary one.
				So(gsMock.publisCalls, ShouldResemble, []publishCall{
					{
						dst:    "/bucket/store/SHA256/5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
						src:    "/bucket/tmp_path/1454472306_1",
						srcGen: 42,
					},
				})
				So(gsMock.deleteCalls, ShouldResemble, []string{"/bucket/tmp_path/1454472306_1"})

				// Caller sees the file is published now.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId: op.OperationId,
					Status:      api.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
					},
				})
			})

			Convey("Publish transient error", func() {
				gsMock.publishErr = errors.Reason("blarg").Tag(transient.Tag).Err()

				// Execute the pending verification task.
				err := impl.verifyUploadTask(ctx, t[0].Payload.(*tasks.VerifyUpload))
				So(transient.Tag.In(err), ShouldBeTrue)

				// Didn't delete anything.
				So(len(gsMock.deleteCalls), ShouldEqual, 0)

				// Caller sees the file is still being verified.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId: op.OperationId,
					Status:      api.UploadStatus_VERIFYING,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				})
			})

			Convey("Publish fatal error", func() {
				gsMock.publishErr = errors.Reason("blarg").Err()

				// Execute the pending verification task.
				err := impl.verifyUploadTask(ctx, t[0].Payload.(*tasks.VerifyUpload))
				So(err, ShouldErrLike, "failed to publish the verified file")
				So(transient.Tag.In(err), ShouldBeFalse)

				// Deleted the temp file.
				So(gsMock.deleteCalls, ShouldResemble, []string{"/bucket/tmp_path/1454472306_1"})

				// Caller is notified about the error.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId:  op.OperationId,
					Status:       api.UploadStatus_ERRORED,
					UploadUrl:    "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					ErrorMessage: "Verification failed - failed to publish the verified file: blarg",
				})
			})
		})

		Convey("Without force hash, known expected hash", func() {
			// Initiate an upload to get operation ID.
			op, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
				Object: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA256,
					HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
				},
			})
			So(err, ShouldBeNil)
			So(op, ShouldResemble, &api.UploadOperation{
				OperationId: op.OperationId,
				Status:      api.UploadStatus_UPLOADING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			})

			// Pretend we've uploaded 5 bytes.
			gsMock.files["/bucket/tmp_path/1454472306_1"] = "12345"

			// Kick off the verification.
			op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
				UploadOperationId: op.OperationId,
			})
			So(err, ShouldBeNil)
			So(op, ShouldResemble, &api.UploadOperation{
				OperationId: op.OperationId,
				Status:      api.UploadStatus_VERIFYING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			})

			// Posted the verification task.
			t := tq.GetScheduledTasks()
			So(len(t), ShouldEqual, 1)
			So(t[0].Payload, ShouldResemble, &tasks.VerifyUpload{UploadOperationId: 1})

			Convey("Successful verification", func() {
				// Execute the pending verification task.
				So(impl.verifyUploadTask(ctx, t[0].Payload.(*tasks.VerifyUpload)), ShouldBeNil)

				// Published the verified file, deleted the temporary one.
				So(gsMock.publisCalls, ShouldResemble, []publishCall{
					{
						dst:    "/bucket/store/SHA256/5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
						src:    "/bucket/tmp_path/1454472306_1",
						srcGen: 42,
					},
				})
				So(gsMock.deleteCalls, ShouldResemble, []string{"/bucket/tmp_path/1454472306_1"})

				// Caller sees the file is published now.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId: op.OperationId,
					Status:      api.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
					},
				})
			})

			Convey("Failed verification", func() {
				// Pretend we've uploaded something not expected.
				gsMock.files["/bucket/tmp_path/1454472306_1"] = "123456"

				err := impl.verifyUploadTask(ctx, t[0].Payload.(*tasks.VerifyUpload))
				So(err, ShouldErrLike, "expected SHA256 to be")
				So(transient.Tag.In(err), ShouldBeFalse)

				// The temp file is deleted.
				So(gsMock.deleteCalls, ShouldResemble, []string{"/bucket/tmp_path/1454472306_1"})

				// Caller is notified about the error.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId: op.OperationId,
					Status:      api.UploadStatus_ERRORED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					ErrorMessage: "Verification failed - expected SHA256 to be " +
						"5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5, " +
						"got 8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92",
				})
			})

			Convey("Published file already exists", func() {
				gsMock.exists = true

				// Execute the pending verification task.
				So(impl.verifyUploadTask(ctx, t[0].Payload.(*tasks.VerifyUpload)), ShouldBeNil)

				// No 'Publish' calls, unnecessary.
				So(len(gsMock.publisCalls), ShouldEqual, 0)
				// Deleted the temp file.
				So(gsMock.deleteCalls, ShouldResemble, []string{"/bucket/tmp_path/1454472306_1"})

				// Caller sees the file is published now.
				op, err = impl.FinishUpload(ctx, &api.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				So(err, ShouldBeNil)
				So(op, ShouldResemble, &api.UploadOperation{
					OperationId: op.OperationId,
					Status:      api.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA256,
						HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
					},
				})
			})
		})

		Convey("Bad force_hash field", func() {
			_, err := impl.FinishUpload(ctx, &api.FinishUploadRequest{
				ForceHash: &api.ObjectRef{
					HashAlgo: 1234,
				},
			})
			So(grpc.Code(err), ShouldEqual, codes.InvalidArgument)
			So(err, ShouldErrLike, "bad 'force_hash' field")
		})

		Convey("Bad operation_id field", func() {
			_, err := impl.FinishUpload(ctx, &api.FinishUploadRequest{
				UploadOperationId: "zzz",
			})
			So(grpc.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such upload operation")
		})
	})
}

func TestCancelUpload(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		const (
			uploadURL  = "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1"
			tempGSPath = "/bucket/tmp_path/1454472306_1"
		)

		ctx, gsMock, tq, impl := storageMocks()

		// Initiate an upload to get operation ID.
		op, err := impl.BeginUpload(ctx, &api.BeginUploadRequest{
			HashAlgo: api.HashAlgo_SHA256,
		})
		So(err, ShouldBeNil)
		So(op, ShouldResemble, &api.UploadOperation{
			OperationId: op.OperationId,
			Status:      api.UploadStatus_UPLOADING,
			UploadUrl:   uploadURL,
		})

		Convey("Cancel right away", func() {
			op, err := impl.CancelUpload(ctx, &api.CancelUploadRequest{
				UploadOperationId: op.OperationId,
			})
			So(err, ShouldBeNil)
			So(op, ShouldResemble, &api.UploadOperation{
				OperationId: op.OperationId,
				Status:      api.UploadStatus_CANCELED,
				UploadUrl:   uploadURL,
			})

			// Should create the TQ task to cleanup.
			t := tq.GetScheduledTasks()
			So(t, ShouldHaveLength, 1)
			So(t[0].Payload, ShouldResemble, &tasks.CleanupUpload{
				UploadOperationId: 1,
				UploadUrl:         uploadURL,
				PathToCleanup:     tempGSPath,
			})

			// Cancel again. Noop, same single task in the queue.
			op2, err := impl.CancelUpload(ctx, &api.CancelUploadRequest{
				UploadOperationId: op.OperationId,
			})
			So(err, ShouldBeNil)
			So(op2, ShouldResemble, op)
			So(tq.GetScheduledTasks(), ShouldHaveLength, 1)

			// Execute the pending task.
			So(impl.cleanupUploadTask(ctx, t[0].Payload.(*tasks.CleanupUpload)), ShouldBeNil)

			// It canceled the session and deleted the file.
			So(gsMock.cancelUploadCalls, ShouldResemble, []string{uploadURL})
			So(gsMock.deleteCalls, ShouldResemble, []string{tempGSPath})
		})

		Convey("Cancel after finishing", func() {
			_, err := impl.FinishUpload(ctx, &api.FinishUploadRequest{
				UploadOperationId: op.OperationId,
			})
			So(err, ShouldBeNil)

			_, err = impl.CancelUpload(ctx, &api.CancelUploadRequest{
				UploadOperationId: op.OperationId,
			})
			So(grpc.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the operation is in state VERIFYING and can't be canceled")
		})
	})
}
