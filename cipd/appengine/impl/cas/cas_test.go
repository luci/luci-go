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
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/appengine/impl/cas/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/cas/upload"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/settings"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	// Using transactional datastore TQ tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
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
		settings: &settings.Settings{StorageGSPath: "/bucket/path"},
		getGS:    func(context.Context, string) gs.GoogleStorage { return gsMock },
	}

	ftt.Run("OK", t, func(t *ftt.Test) {
		r, err := impl.GetReader(ctx, &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: sha256,
		}, "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, r, should.NotBeNilInterface)

		body := make([]byte, 100)
		l, err := r.ReadAt(body, 0)
		assert.Loosely(t, err, should.Equal(io.EOF))
		body = body[:l]
		assert.Loosely(t, string(body), should.Equal("zzz body"))
	})

	ftt.Run("Bad object ref", t, func(t *ftt.Test) {
		_, err := impl.GetReader(ctx, &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: "zzz",
		}, "")
		assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("bad ref"))
	})

	ftt.Run("No such file", t, func(t *ftt.Test) {
		_, err := impl.GetReader(ctx, &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: strings.Repeat("b", 64),
		}, "")
		assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
		assert.Loosely(t, err, should.ErrLike("can't read the object"))
	})
}

func TestGetObjectURL(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	var signErr error
	impl := storageImpl{
		settings: &settings.Settings{StorageGSPath: "/bucket/path"},
		getGS:    func(context.Context, string) gs.GoogleStorage { return &testutil.NoopGoogleStorage{} },
		getSignedURL: func(ctx context.Context, signer signerFactory, gs gs.GoogleStorage, params *signedURLParams) (string, uint64, error) {
			return "http//signed.example.com" + params.GsPath + "?f=" + params.Filename, 123, signErr
		},
	}

	ftt.Run("OK", t, func(t *ftt.Test) {
		resp, err := impl.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
			Object: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
			DownloadFilename: "file.name",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp, should.Resemble(&caspb.ObjectURL{
			SignedUrl: "http//signed.example.com/bucket/path/SHA256/" +
				strings.Repeat("a", 64) + "?f=file.name",
		}))
	})

	ftt.Run("Bad object ref", t, func(t *ftt.Test) {
		_, err := impl.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
			Object: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: "zzz",
			},
		})
		assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("bad 'object' field"))
	})

	ftt.Run("Bad filename", t, func(t *ftt.Test) {
		_, err := impl.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
			Object: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
			DownloadFilename: "abc\ndef",
		})
		assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
		assert.Loosely(t, err, should.ErrLike("bad 'download_filename' field"))
	})

	ftt.Run("No such file", t, func(t *ftt.Test) {
		signErr = grpcutil.NotFoundTag.Apply(errors.New("blah"))
		_, err := impl.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
			Object: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
		})
		assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
		assert.Loosely(t, err, should.ErrLike("blah"))
	})

	ftt.Run("Internal error", t, func(t *ftt.Test) {
		signErr = errors.New("internal")
		_, err := impl.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
			Object: &caspb.ObjectRef{
				HashAlgo:  caspb.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
			},
		})
		assert.Loosely(t, status.Code(err), should.Equal(codes.Unknown))
		assert.Loosely(t, err, should.ErrLike("internal"))
	})
}

type mockedGS struct {
	testutil.NoopGoogleStorage

	everythingExists bool
	files            map[string]string

	accessErr   error
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

func (m *mockedGS) Size(ctx context.Context, path string) (size uint64, exists bool, err error) {
	if m.accessErr != nil {
		return 0, false, m.accessErr
	}
	if body, ok := m.files[path]; ok {
		return uint64(len(body)), true, nil
	}
	return 0, m.everythingExists, nil
}

func (m *mockedGS) Exists(ctx context.Context, path string) (bool, error) {
	_, exists, err := m.Size(ctx, path)
	return exists, err
}

func (m *mockedGS) StartUpload(ctx context.Context, path string) (string, error) {
	return "http://upload-url.example.com/for/+" + path, nil
}

func (m *mockedGS) CancelUpload(ctx context.Context, uploadURL string) error {
	m.cancelUploadCalls = append(m.cancelUploadCalls, uploadURL)
	return nil
}

func (m *mockedGS) Reader(ctx context.Context, path string, gen, minSpeed int64) (gs.Reader, error) {
	if m.accessErr != nil {
		return nil, m.accessErr
	}
	if body, ok := m.files[path]; ok {
		return mockedGSReader{Reader: strings.NewReader(body)}, nil
	}
	if m.everythingExists {
		return mockedGSReader{Reader: strings.NewReader("")}, nil
	}
	return nil, grpcutil.NotFoundTag.Apply(errors.Fmt("file %q is missing", path))
}

func (m *mockedGS) Publish(ctx context.Context, dst, src string, srcGen int64) error {
	if m.publisCalls == nil {
		panic("didn't expect Publish calls")
	}
	m.publisCalls = append(m.publisCalls, publishCall{dst, src, srcGen})
	return m.publishErr
}

func (m *mockedGS) Delete(ctx context.Context, path string) error {
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

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx, gsMock, _, _, impl := storageMocks()

		t.Run("Success (no Object)", func(t *ftt.Test) {
			resp, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				HashAlgo: caspb.HashAlgo_SHA256,
			})
			assert.Loosely(t, err, should.BeNil)

			// ID can be decoded back.
			opID, err := upload.UnwrapOpID(ctx, resp.OperationId, testutil.TestUser)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, opID, should.Equal(1))

			// Rest of the response looks OK too.
			resp.OperationId = ""
			assert.Loosely(t, resp, should.Resemble(&caspb.UploadOperation{
				UploadUrl: "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				Status:    caspb.UploadStatus_UPLOADING,
			}))

			// Created the entity.
			op := upload.Operation{ID: 1}
			assert.Loosely(t, datastore.Get(ctx, &op), should.BeNil)

			assert.Loosely(t, op.CreatedTS.Equal(testutil.TestTime), should.BeTrue)
			op.CreatedTS = time.Time{}
			assert.Loosely(t, op.UpdatedTS.Equal(testutil.TestTime), should.BeTrue)
			op.UpdatedTS = time.Time{}

			assert.Loosely(t, op, should.Resemble(upload.Operation{
				ID:         1,
				Status:     caspb.UploadStatus_UPLOADING,
				TempGSPath: "/bucket/tmp_path/1454472306_1",
				UploadURL:  "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				HashAlgo:   caspb.HashAlgo_SHA256,
				CreatedBy:  testutil.TestUser,
			}))
		})

		t.Run("Success (Object is not present in the store)", func(t *ftt.Test) {
			resp, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				Object: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
				},
			})
			assert.Loosely(t, err, should.BeNil)

			resp.OperationId = ""
			assert.Loosely(t, resp, should.Resemble(&caspb.UploadOperation{
				UploadUrl: "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				Status:    caspb.UploadStatus_UPLOADING,
			}))

			op := upload.Operation{ID: 1}
			assert.Loosely(t, datastore.Get(ctx, &op), should.BeNil)
			assert.Loosely(t, op, should.Resemble(upload.Operation{
				ID:         1,
				Status:     caspb.UploadStatus_UPLOADING,
				TempGSPath: "/bucket/tmp_path/1454472306_1",
				UploadURL:  "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				HashAlgo:   caspb.HashAlgo_SHA256,
				HexDigest:  strings.Repeat("a", 64),
				CreatedBy:  testutil.TestUser,
				CreatedTS:  op.CreatedTS,
				UpdatedTS:  op.UpdatedTS,
			}))
		})

		t.Run("Object already exists", func(t *ftt.Test) {
			gsMock.everythingExists = true
			_, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				Object: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.AlreadyExists))
		})

		t.Run("Bad object", func(t *ftt.Test) {
			_, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				Object: &caspb.ObjectRef{
					HashAlgo: 1234,
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'object'"))
		})

		t.Run("Bad hash_algo", func(t *ftt.Test) {
			_, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				HashAlgo: 1234,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'hash_algo'"))
		})

		t.Run("Mismatch in hash_algo", func(t *ftt.Test) {
			_, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				Object: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: strings.Repeat("a", 64),
				},
				HashAlgo: 333, // something else
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("'hash_algo' and 'object.hash_algo' do not match"))
		})
	})
}

type verificationLogs struct {
	m    sync.Mutex
	logs []*caspb.VerificationLogEntry
}

func (v *verificationLogs) submitLog(_ context.Context, entry *caspb.VerificationLogEntry) {
	v.m.Lock()
	defer v.m.Unlock()
	v.logs = append(v.logs, proto.Clone(entry).(*caspb.VerificationLogEntry))
}

func (v *verificationLogs) last() *caspb.VerificationLogEntry {
	v.m.Lock()
	defer v.m.Unlock()
	if len(v.logs) == 0 {
		return nil
	}
	return v.logs[len(v.logs)-1]
}

func storageMocks() (context.Context, *mockedGS, *tqtesting.Scheduler, *verificationLogs, *storageImpl) {
	ctx, _, _ := testutil.TestingContext()
	ctx = secrets.Use(ctx, &testsecrets.Store{})

	dispatcher := &tq.Dispatcher{}
	ctx, sched := tq.TestingContext(ctx, dispatcher)

	gsMock := &mockedGS{
		files:       map[string]string{},
		publisCalls: []publishCall{},
		deleteCalls: []string{},
	}

	logs := &verificationLogs{}

	impl := &storageImpl{
		tq: dispatcher,
		settings: &settings.Settings{
			StorageGSPath: "/bucket/store",
			TempGSPath:    "/bucket/tmp_path",
		},
		getGS:     func(context.Context, string) gs.GoogleStorage { return gsMock },
		submitLog: logs.submitLog,
	}
	impl.registerTasks()

	return ctx, gsMock, sched, logs, impl
}

func TestFinishUpload(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx, gsMock, tq, verificationLogs, impl := storageMocks()

		t.Run("With force hash", func(t *ftt.Test) {
			// Initiate an upload to get operation ID.
			op, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				HashAlgo: caspb.HashAlgo_SHA256,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
				OperationId: op.OperationId,
				Status:      caspb.UploadStatus_UPLOADING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			}))

			// Pretend we've uploaded 5 bytes.
			gsMock.files["/bucket/tmp_path/1454472306_1"] = "12345"

			t.Run("Success", func(t *ftt.Test) {
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
					ForceHash: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId: op.OperationId,
					Status:      caspb.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				}))

				// Published the file, deleted the temporary one.
				assert.Loosely(t, gsMock.publisCalls, should.Resemble([]publishCall{
					{
						dst:    "/bucket/store/SHA256/" + strings.Repeat("a", 64),
						src:    "/bucket/tmp_path/1454472306_1",
						srcGen: -1,
					},
				}))
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))
			})

			t.Run("Publish transient error", func(t *ftt.Test) {
				gsMock.publishErr = transient.Tag.Apply(errors.New("blarg"))
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
					ForceHash: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})
				assert.Loosely(t, status.Code(err), should.Equal(codes.Internal))

				// Status untouched.
				entity := upload.Operation{ID: 1}
				assert.Loosely(t, datastore.Get(ctx, &entity), should.BeNil)
				assert.Loosely(t, entity.Status, should.Equal(caspb.UploadStatus_UPLOADING))
			})

			t.Run("Publish fatal error", func(t *ftt.Test) {
				gsMock.publishErr = errors.New("blarg")
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
					ForceHash: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: strings.Repeat("a", 64),
					},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId:  op.OperationId,
					Status:       caspb.UploadStatus_ERRORED,
					UploadUrl:    "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					ErrorMessage: "Failed to publish the object - blarg",
				}))
			})
		})

		t.Run("Without force hash, unknown expected hash", func(t *ftt.Test) {
			// Initiate an upload to get operation ID.
			op, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				HashAlgo: caspb.HashAlgo_SHA256,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
				OperationId: op.OperationId,
				Status:      caspb.UploadStatus_UPLOADING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			}))

			// Pretend we've uploaded 5 bytes.
			gsMock.files["/bucket/tmp_path/1454472306_1"] = "12345"

			// Kick off the verification.
			op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
				UploadOperationId: op.OperationId,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
				OperationId: op.OperationId,
				Status:      caspb.UploadStatus_VERIFYING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			}))

			// Posted the verification task.
			tqTasks := tq.Tasks()
			assert.Loosely(t, tqTasks, should.HaveLength(1))
			assert.Loosely(t, tqTasks[0].Payload, should.Resemble(&tasks.VerifyUpload{UploadOperationId: 1}))

			t.Run("Retrying FinishUpload does nothing", func(t *ftt.Test) {
				// Retrying the call does nothing.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op.Status, should.Equal(caspb.UploadStatus_VERIFYING))

				// Still only 1 task in the queue.
				assert.Loosely(t, tq.Tasks(), should.HaveLength(1))
			})

			t.Run("Successful verification", func(t *ftt.Test) {
				// Execute the pending verification task.
				assert.Loosely(t, impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload)), should.BeNil)

				// Published the verified file, deleted the temporary one.
				assert.Loosely(t, gsMock.publisCalls, should.Resemble([]publishCall{
					{
						dst:    "/bucket/store/SHA256/5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
						src:    "/bucket/tmp_path/1454472306_1",
						srcGen: 42,
					},
				}))
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))

				// Caller sees the file is published now.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId: op.OperationId,
					Status:      caspb.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
					},
				}))

				// There's a log entry.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					VerifiedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					FileSize:           5,
					VerificationSpeed:  5000, // this is fake, our time is frozen
					Outcome:            "PUBLISHED",
				}))
			})

			t.Run("Publish transient error", func(t *ftt.Test) {
				gsMock.publishErr = transient.Tag.Apply(errors.New("blarg"))

				// Execute the pending verification task.
				err := impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)

				// Didn't delete anything.
				assert.Loosely(t, len(gsMock.deleteCalls), should.BeZero)

				// Caller sees the file is still being verified.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId: op.OperationId,
					Status:      caspb.UploadStatus_VERIFYING,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
				}))

				// There's a log entry.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					VerifiedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					FileSize:           5,
					VerificationSpeed:  5000, // this is fake, our time is frozen
					Outcome:            "ERRORED",
					Error:              "Transient error: failed to publish the verified file: blarg",
					TransientError:     true,
				}))
			})

			t.Run("Publish fatal error", func(t *ftt.Test) {
				gsMock.publishErr = errors.New("blarg")

				// Execute the pending verification task.
				err := impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload))
				assert.Loosely(t, err, should.ErrLike("failed to publish the verified file"))
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)

				// Deleted the temp file.
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))

				// Caller is notified about the error.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId:  op.OperationId,
					Status:       caspb.UploadStatus_ERRORED,
					UploadUrl:    "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					ErrorMessage: "Verification failed: failed to publish the verified file: blarg",
				}))

				// There's a log entry.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					VerifiedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					FileSize:           5,
					VerificationSpeed:  5000, // this is fake, our time is frozen
					Outcome:            "ERRORED",
					Error:              "Verification failed: failed to publish the verified file: blarg",
				}))
			})
		})

		t.Run("Without force hash, known expected hash", func(t *ftt.Test) {
			// Initiate an upload to get operation ID.
			op, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
				Object: &caspb.ObjectRef{
					HashAlgo:  caspb.HashAlgo_SHA256,
					HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
				OperationId: op.OperationId,
				Status:      caspb.UploadStatus_UPLOADING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			}))

			// Pretend we've uploaded 5 bytes.
			gsMock.files["/bucket/tmp_path/1454472306_1"] = "12345"

			// Kick off the verification.
			op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
				UploadOperationId: op.OperationId,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
				OperationId: op.OperationId,
				Status:      caspb.UploadStatus_VERIFYING,
				UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
			}))

			// Posted the verification task.
			tqTasks := tq.Tasks()
			assert.Loosely(t, tqTasks, should.HaveLength(1))
			assert.Loosely(t, tqTasks[0].Payload, should.Resemble(&tasks.VerifyUpload{UploadOperationId: 1}))

			t.Run("Successful verification", func(t *ftt.Test) {
				// Execute the pending verification task.
				assert.Loosely(t, impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload)), should.BeNil)

				// Published the verified file, deleted the temporary one.
				assert.Loosely(t, gsMock.publisCalls, should.Resemble([]publishCall{
					{
						dst:    "/bucket/store/SHA256/5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
						src:    "/bucket/tmp_path/1454472306_1",
						srcGen: 42,
					},
				}))
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))

				// Caller sees the file is published now.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId: op.OperationId,
					Status:      caspb.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
					},
				}))

				// There's a log entry.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					ExpectedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					VerifiedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					FileSize:           5,
					VerificationSpeed:  5000, // this is fake, our time is frozen
					Outcome:            "PUBLISHED",
				}))
			})

			t.Run("Failed verification", func(t *ftt.Test) {
				// Pretend we've uploaded something not expected.
				gsMock.files["/bucket/tmp_path/1454472306_1"] = "123456"

				err := impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload))
				assert.Loosely(t, err, should.ErrLike("expected SHA256 to be"))
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)

				// The temp file is deleted.
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))

				// Caller is notified about the error.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId: op.OperationId,
					Status:      caspb.UploadStatus_ERRORED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					ErrorMessage: "Verification failed: expected SHA256 to be " +
						"5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5, " +
						"got 8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92",
				}))
			})

			t.Run("Published file already exists", func(t *ftt.Test) {
				gsMock.everythingExists = true

				// Execute the pending verification task.
				assert.Loosely(t, impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload)), should.BeNil)

				// No 'Publish' calls, unnecessary.
				assert.Loosely(t, len(gsMock.publisCalls), should.BeZero)
				// Deleted the temp file.
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))

				// Caller sees the file is published now.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
					OperationId: op.OperationId,
					Status:      caspb.UploadStatus_PUBLISHED,
					UploadUrl:   "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1",
					Object: &caspb.ObjectRef{
						HashAlgo:  caspb.HashAlgo_SHA256,
						HexDigest: "5994471abb01112afcc18159f6cc74b4f511b99806da59b3caf5a9c173cacfc5",
					},
				}))

				// The log entry indicates the verification was skipped.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					ExpectedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					VerifiedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					Outcome:            "PUBLISHED",
					Existed:            true,
				}))
			})

			t.Run("Transient error checking existing published file", func(t *ftt.Test) {
				gsMock.accessErr = transient.Tag.Apply(fmt.Errorf("boom"))

				// Execute the pending verification task.
				err := impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)

				// Didn't delete anything.
				assert.Loosely(t, len(gsMock.deleteCalls), should.BeZero)

				// Left the operation in VERIFYING state.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op.Status, should.Equal(caspb.UploadStatus_VERIFYING))

				// Recorded the transient error.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					ExpectedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					Outcome:            "ERRORED",
					Error:              "Transient error: failed to check the presence of the destination file: boom",
					TransientError:     true,
				}))
			})

			t.Run("Fatal error checking existing published file", func(t *ftt.Test) {
				gsMock.accessErr = fmt.Errorf("boom")

				// Execute the pending verification task.
				err := impl.verifyUploadTask(ctx, tqTasks[0].Payload.(*tasks.VerifyUpload))
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)

				// Deleted the temp file.
				assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{"/bucket/tmp_path/1454472306_1"}))

				// The operation is failed.
				op, err = impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
					UploadOperationId: op.OperationId,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, op.Status, should.Equal(caspb.UploadStatus_ERRORED))

				// Recorded the error.
				assert.Loosely(t, verificationLogs.last(), should.Resemble(&caspb.VerificationLogEntry{
					OperationId:        1,
					TraceId:            testutil.TestRequestID.String(),
					InitiatedBy:        string(testutil.TestUser),
					TempGsPath:         "/bucket/tmp_path/1454472306_1",
					ExpectedInstanceId: "WZRHGrsBESr8wYFZ9sx0tPURuZgG2lmzyvWpwXPKz8UC",
					Submitted:          testutil.TestTime.UnixNano() / 1000,
					Started:            testutil.TestTime.UnixNano() / 1000,
					Finished:           testutil.TestTime.UnixNano() / 1000,
					Outcome:            "ERRORED",
					Error:              "Verification failed: failed to check the presence of the destination file: boom",
				}))
			})
		})

		t.Run("Bad force_hash field", func(t *ftt.Test) {
			_, err := impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
				ForceHash: &caspb.ObjectRef{
					HashAlgo: 1234,
				},
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad 'force_hash' field"))
		})

		t.Run("Bad operation_id field", func(t *ftt.Test) {
			_, err := impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
				UploadOperationId: "zzz",
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such upload operation"))
		})
	})
}

func TestCancelUpload(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		const (
			uploadURL  = "http://upload-url.example.com/for/+/bucket/tmp_path/1454472306_1"
			tempGSPath = "/bucket/tmp_path/1454472306_1"
		)

		ctx, gsMock, tq, _, impl := storageMocks()

		// Initiate an upload to get operation ID.
		op, err := impl.BeginUpload(ctx, &caspb.BeginUploadRequest{
			HashAlgo: caspb.HashAlgo_SHA256,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
			OperationId: op.OperationId,
			Status:      caspb.UploadStatus_UPLOADING,
			UploadUrl:   uploadURL,
		}))

		t.Run("Cancel right away", func(t *ftt.Test) {
			op, err = impl.CancelUpload(ctx, &caspb.CancelUploadRequest{
				UploadOperationId: op.OperationId,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op, should.Resemble(&caspb.UploadOperation{
				OperationId: op.OperationId,
				Status:      caspb.UploadStatus_CANCELED,
				UploadUrl:   uploadURL,
			}))

			// Should create the TQ task to cleanup.
			tqTasks := tq.Tasks()
			assert.Loosely(t, tqTasks, should.HaveLength(1))
			assert.Loosely(t, tqTasks[0].Payload, should.Resemble(&tasks.CleanupUpload{
				UploadOperationId: 1,
				UploadUrl:         uploadURL,
				PathToCleanup:     tempGSPath,
			}))

			// Cancel again. Noop, same single task in the queue.
			op2, err := impl.CancelUpload(ctx, &caspb.CancelUploadRequest{
				UploadOperationId: op.OperationId,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op2, should.Resemble(op))
			assert.Loosely(t, tq.Tasks(), should.HaveLength(1))

			// Execute the pending task.
			assert.Loosely(t, impl.cleanupUploadTask(ctx, tqTasks[0].Payload.(*tasks.CleanupUpload)), should.BeNil)

			// It canceled the session and deleted the file.
			assert.Loosely(t, gsMock.cancelUploadCalls, should.Resemble([]string{uploadURL}))
			assert.Loosely(t, gsMock.deleteCalls, should.Resemble([]string{tempGSPath}))
		})

		t.Run("Cancel after finishing", func(t *ftt.Test) {
			_, err := impl.FinishUpload(ctx, &caspb.FinishUploadRequest{
				UploadOperationId: op.OperationId,
			})
			assert.Loosely(t, err, should.BeNil)

			_, err = impl.CancelUpload(ctx, &caspb.CancelUploadRequest{
				UploadOperationId: op.OperationId,
			})
			assert.Loosely(t, status.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the operation is in state VERIFYING and can't be canceled"))
		})
	})
}
