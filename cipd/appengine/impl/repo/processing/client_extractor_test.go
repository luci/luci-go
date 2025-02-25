// Copyright 2018 The LUCI Authors.
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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"
)

func phonyHexDigest(algo api.HashAlgo, letter string) string {
	return strings.Repeat(letter, map[api.HashAlgo]int{
		api.HashAlgo_SHA1:   40,
		api.HashAlgo_SHA256: 64,
	}[algo])
}

func phonyInstanceID(algo api.HashAlgo, letter string) string {
	return common.ObjectRefToInstanceID(&api.ObjectRef{
		HashAlgo:  algo,
		HexDigest: phonyHexDigest(algo, letter),
	})
}

func instance(ctx context.Context, pkg string, algo api.HashAlgo) *model.Instance {
	return &model.Instance{
		InstanceID: phonyInstanceID(algo, "a"),
		Package:    model.PackageKey(ctx, pkg),
	}
}

func hexDigest(algo api.HashAlgo, body string) string {
	h := common.MustNewHash(algo)
	if _, err := h.Write([]byte(body)); err != nil {
		panic(err)
	}
	return common.HexDigest(h)
}

func TestGetClientPackage(t *testing.T) {
	t.Parallel()

	ftt.Run("works", t, func(t *ftt.Test) {
		pkg, err := GetClientPackage("linux-amd64")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pkg, should.Equal("infra/tools/cipd/linux-amd64"))
	})

	ftt.Run("fails", t, func(t *ftt.Test) {
		_, err := GetClientPackage("../sneaky")
		assert.Loosely(t, err, should.ErrLike("invalid package name"))
	})
}

func TestClientExtractor(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	originalBody := strings.Repeat("01234567", 8960)
	expectedDigests := map[string]string{
		"SHA1":   hexDigest(api.HashAlgo_SHA1, originalBody),
		"SHA256": hexDigest(api.HashAlgo_SHA256, originalBody),
	}

	instSHA256 := instance(ctx, "infra/tools/cipd/linux-amd64", api.HashAlgo_SHA256)
	instSHA1 := instance(ctx, "infra/tools/cipd/linux-amd64", api.HashAlgo_SHA1)

	goodPkg := packageReader(map[string]string{"cipd": originalBody})

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

		ce := ClientExtractor{
			CAS: &cas,

			uploader:   func(ctx context.Context, size int64, uploadURL string) io.Writer { return uploader },
			bufferSize: 64 * 1024,
		}

		t.Run("Happy path, SHA256", func(t *ftt.Test) {
			res, err := ce.Run(ctx, instSHA256, goodPkg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.BeNil)

			r := res.Result.(ClientExtractorResult)
			assert.Loosely(t, r.ClientBinary.Size, should.Equal(len(originalBody)))
			assert.Loosely(t, r.ClientBinary.HashAlgo, should.Equal("SHA256"))
			assert.Loosely(t, r.ClientBinary.HashDigest, should.Equal(expectedDigests["SHA256"]))
			assert.Loosely(t, r.ClientBinary.AllHashDigests, should.Match(expectedDigests))

			assert.Loosely(t, extracted.String(), should.Equal(originalBody))
			assert.Loosely(t, publishedRef, should.Match(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: expectedDigests["SHA256"],
			}))

			// Was written by 64 Kb chunks, NOT 32 Kb (as used by zip.Reader).
			assert.Loosely(t, uploader.calls, should.Match([]int{64 * 1024, 6 * 1024}))
		})

		// TODO(vadimsh): Delete this test once SHA1 uploads are forbidden.
		t.Run("Happy path, SHA1", func(t *ftt.Test) {
			expectedUploadAlgo = api.HashAlgo_SHA1
			res, err := ce.Run(ctx, instSHA1, goodPkg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.BeNil)

			r := res.Result.(ClientExtractorResult)
			assert.Loosely(t, r.ClientBinary.Size, should.Equal(len(originalBody)))
			assert.Loosely(t, r.ClientBinary.HashAlgo, should.Equal("SHA1"))
			assert.Loosely(t, r.ClientBinary.HashDigest, should.Equal(expectedDigests["SHA1"]))
			assert.Loosely(t, r.ClientBinary.AllHashDigests, should.Match(expectedDigests))

			assert.Loosely(t, publishedRef, should.Match(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: expectedDigests["SHA1"],
			}))
		})

		t.Run("No such file in the package", func(t *ftt.Test) {
			res, err := ce.Run(ctx, instSHA256, packageReader(nil))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Err, should.ErrLike(`failed to open the file for reading: no file "cipd" inside the package`))
		})

		t.Run("Internal error when initiating the upload", func(t *ftt.Test) {
			cas.BeginUploadImpl = func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, status.Errorf(codes.Internal, "boo")
			}
			_, err := ce.Run(ctx, instSHA256, goodPkg)
			assert.Loosely(t, err, should.ErrLike(`failed to open a CAS upload: rpc error: code = Internal desc = boo`))
		})

		t.Run("Internal error when finalizing the upload", func(t *ftt.Test) {
			cas.FinishUploadImpl = func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				return nil, status.Errorf(codes.Internal, "boo")
			}
			_, err := ce.Run(ctx, instSHA256, goodPkg)
			assert.Loosely(t, err, should.ErrLike(`failed to finalize the CAS upload: rpc error: code = Internal desc = boo`))
		})

		t.Run("Asked to restart the upload", func(t *ftt.Test) {
			uploader.err = &gs.RestartUploadError{Offset: 124}

			_, err := ce.Run(ctx, instSHA256, goodPkg)
			assert.Loosely(t, err, should.ErrLike(`asked to restart the upload from faraway offset: the upload should be restarted from offset 124`))
			assert.Loosely(t, canceled, should.BeTrue)
		})
	})

	ftt.Run("Applicable works", t, func(t *ftt.Test) {
		ce := ClientExtractor{}

		res, err := ce.Applicable(ctx, instance(ctx, "infra/tools/cipd/linux", api.HashAlgo_SHA256))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeTrue)

		res, err = ce.Applicable(ctx, instance(ctx, "infra/tools/stuff/linux", api.HashAlgo_SHA256))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeFalse)
	})
}

func TestGetResult(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		instRef := &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: phonyHexDigest(api.HashAlgo_SHA256, "a"),
		}

		write := func(res *ClientExtractorResult, err string) {
			r := model.ProcessingResult{
				ProcID: ClientExtractorProcID,
				Instance: datastore.KeyForObj(ctx, &model.Instance{
					InstanceID: common.ObjectRefToInstanceID(instRef),
					Package:    model.PackageKey(ctx, "a/b/c"),
				}),
			}
			if res != nil {
				r.Success = true
				assert.Loosely(t, r.WriteResult(&res), should.BeNil)
			} else {
				r.Error = err
			}
			assert.Loosely(t, datastore.Put(ctx, &r), should.BeNil)
		}

		t.Run("Happy path", func(t *ftt.Test) {
			res := ClientExtractorResult{}
			res.ClientBinary.HashAlgo = "SHA256"
			res.ClientBinary.HashDigest = phonyHexDigest(api.HashAlgo_SHA256, "b")
			res.ClientBinary.AllHashDigests = map[string]string{
				"SHA1":   phonyHexDigest(api.HashAlgo_SHA1, "c"),
				"SHA256": phonyHexDigest(api.HashAlgo_SHA256, "b"),
				"SHA999": strings.Repeat("e", 99), // should silently be skipped
			}
			write(&res, "")

			out, err := GetClientExtractorResult(ctx, &api.Instance{
				Package:  "a/b/c",
				Instance: instRef,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Match(&res))

			ref, err := out.ToObjectRef()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ref, should.Match(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: res.ClientBinary.HashDigest,
			}))

			assert.Loosely(t, out.ObjectRefAliases(), should.Match([]*api.ObjectRef{
				{HashAlgo: api.HashAlgo_SHA1, HexDigest: phonyHexDigest(api.HashAlgo_SHA1, "c")},
				{HashAlgo: api.HashAlgo_SHA256, HexDigest: phonyHexDigest(api.HashAlgo_SHA256, "b")},
			}))
		})

		t.Run("Legacy SHA1 record with no AllHashDigests", func(t *ftt.Test) {
			res := ClientExtractorResult{}
			res.ClientBinary.HashAlgo = "SHA1"
			res.ClientBinary.HashDigest = phonyHexDigest(api.HashAlgo_SHA1, "a")

			ref, err := res.ToObjectRef()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ref, should.Match(&api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: res.ClientBinary.HashDigest,
			}))

			assert.Loosely(t, res.ObjectRefAliases(), should.Match([]*api.ObjectRef{
				{HashAlgo: api.HashAlgo_SHA1, HexDigest: res.ClientBinary.HashDigest},
			}))
		})

		t.Run("Failed processor", func(t *ftt.Test) {
			write(nil, "boom")

			_, err := GetClientExtractorResult(ctx, &api.Instance{
				Package:  "a/b/c",
				Instance: instRef,
			})
			assert.Loosely(t, err, should.ErrLike("boom"))
		})

		t.Run("No result", func(t *ftt.Test) {
			_, err := GetClientExtractorResult(ctx, &api.Instance{
				Package:  "a/b/c",
				Instance: instRef,
			})
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
		})
	})
}
