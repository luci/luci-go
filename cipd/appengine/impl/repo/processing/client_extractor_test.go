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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func packageReader(data map[string]string) *PackageReader {
	buf := bytes.NewReader(testutil.MakeZip(data))
	size := int64(buf.Len())
	r, _ := NewPackageReader(buf, size)
	return r
}

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

	Convey("works", t, func() {
		pkg, err := GetClientPackage("linux-amd64")
		So(err, ShouldBeNil)
		So(pkg, ShouldEqual, "infra/tools/cipd/linux-amd64")
	})

	Convey("fails", t, func() {
		_, err := GetClientPackage("../sneaky")
		So(err, ShouldErrLike, "invalid package name")
	})
}

func TestClientExtractor(t *testing.T) {
	t.Parallel()

	ctx := gaetesting.TestingContext()

	originalBody := strings.Repeat("01234567", 8960)
	expectedDigests := map[string]string{
		"SHA1":   hexDigest(api.HashAlgo_SHA1, originalBody),
		"SHA256": hexDigest(api.HashAlgo_SHA256, originalBody),
	}

	instSHA256 := instance(ctx, "infra/tools/cipd/linux-amd64", api.HashAlgo_SHA256)
	instSHA1 := instance(ctx, "infra/tools/cipd/linux-amd64", api.HashAlgo_SHA1)

	goodPkg := packageReader(map[string]string{"cipd": originalBody})

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

		ce := ClientExtractor{
			CAS: &cas,

			uploader:   func(ctx context.Context, size int64, uploadURL string) io.Writer { return uploader },
			bufferSize: 64 * 1024,
		}

		Convey("Happy path, SHA256", func() {
			res, err := ce.Run(ctx, instSHA256, goodPkg)
			So(err, ShouldBeNil)
			So(res.Err, ShouldBeNil)

			r := res.Result.(ClientExtractorResult)
			So(r.ClientBinary.Size, ShouldEqual, len(originalBody))
			So(r.ClientBinary.HashAlgo, ShouldEqual, "SHA256")
			So(r.ClientBinary.HashDigest, ShouldEqual, expectedDigests["SHA256"])
			So(r.ClientBinary.AllHashDigests, ShouldResemble, expectedDigests)

			So(extracted.String(), ShouldEqual, originalBody)
			So(publishedRef, ShouldResembleProto, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: expectedDigests["SHA256"],
			})

			// Was written by 64 Kb chunks, NOT 32 Kb (as used by zip.Reader).
			So(uploader.calls, ShouldResemble, []int{64 * 1024, 6 * 1024})
		})

		// TODO(vadimsh): Delete this test once SHA1 uploads are forbidden.
		Convey("Happy path, SHA1", func() {
			expectedUploadAlgo = api.HashAlgo_SHA1
			res, err := ce.Run(ctx, instSHA1, goodPkg)
			So(err, ShouldBeNil)
			So(res.Err, ShouldBeNil)

			r := res.Result.(ClientExtractorResult)
			So(r.ClientBinary.Size, ShouldEqual, len(originalBody))
			So(r.ClientBinary.HashAlgo, ShouldEqual, "SHA1")
			So(r.ClientBinary.HashDigest, ShouldEqual, expectedDigests["SHA1"])
			So(r.ClientBinary.AllHashDigests, ShouldResemble, expectedDigests)

			So(publishedRef, ShouldResembleProto, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: expectedDigests["SHA1"],
			})
		})

		Convey("No such file in the package", func() {
			res, err := ce.Run(ctx, instSHA256, packageReader(nil))
			So(err, ShouldBeNil)
			So(res.Err, ShouldErrLike, `failed to open the file for reading: no file "cipd" inside the package`)
		})

		Convey("Internal error when initiating the upload", func() {
			cas.BeginUploadImpl = func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.Internal, "boo")
			}
			_, err := ce.Run(ctx, instSHA256, goodPkg)
			So(err, ShouldErrLike, `failed to open a CAS upload: rpc error: code = Internal desc = boo`)
		})

		Convey("Internal error when finalizing the upload", func() {
			cas.FinishUploadImpl = func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.Internal, "boo")
			}
			_, err := ce.Run(ctx, instSHA256, goodPkg)
			So(err, ShouldErrLike, `failed to finalize the CAS upload: rpc error: code = Internal desc = boo`)
		})

		Convey("Asked to restart the upload", func() {
			uploader.err = &gs.RestartUploadError{Offset: 124}

			_, err := ce.Run(ctx, instSHA256, goodPkg)
			So(err, ShouldErrLike, `asked to restart the upload from faraway offset: the upload should be restarted from offset 124`)
			So(canceled, ShouldBeTrue)
		})
	})

	Convey("Applicable works", t, func() {
		ce := ClientExtractor{}
		So(ce.Applicable(instance(ctx, "infra/tools/cipd/linux", api.HashAlgo_SHA256)), ShouldBeTrue)
		So(ce.Applicable(instance(ctx, "infra/tools/stuff/linux", api.HashAlgo_SHA256)), ShouldBeFalse)
	})
}

func TestGetResult(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := gaetesting.TestingContext()

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
				So(r.WriteResult(&res), ShouldBeNil)
			} else {
				r.Error = err
			}
			So(datastore.Put(ctx, &r), ShouldBeNil)
		}

		Convey("Happy path", func() {
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
			So(err, ShouldBeNil)
			So(out, ShouldResemble, &res)

			ref, err := out.ToObjectRef()
			So(err, ShouldBeNil)
			So(ref, ShouldResemble, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: res.ClientBinary.HashDigest,
			})

			So(out.ObjectRefAliases(), ShouldResemble, []*api.ObjectRef{
				{HashAlgo: api.HashAlgo_SHA1, HexDigest: phonyHexDigest(api.HashAlgo_SHA1, "c")},
				{HashAlgo: api.HashAlgo_SHA256, HexDigest: phonyHexDigest(api.HashAlgo_SHA256, "b")},
			})
		})

		Convey("Legacy SHA1 record with no AllHashDigests", func() {
			res := ClientExtractorResult{}
			res.ClientBinary.HashAlgo = "SHA1"
			res.ClientBinary.HashDigest = phonyHexDigest(api.HashAlgo_SHA1, "a")

			ref, err := res.ToObjectRef()
			So(err, ShouldBeNil)
			So(ref, ShouldResemble, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: res.ClientBinary.HashDigest,
			})

			So(res.ObjectRefAliases(), ShouldResemble, []*api.ObjectRef{
				{HashAlgo: api.HashAlgo_SHA1, HexDigest: res.ClientBinary.HashDigest},
			})
		})

		Convey("Failed processor", func() {
			write(nil, "boom")

			_, err := GetClientExtractorResult(ctx, &api.Instance{
				Package:  "a/b/c",
				Instance: instRef,
			})
			So(err, ShouldErrLike, "boom")
		})

		Convey("No result", func() {
			_, err := GetClientExtractorResult(ctx, &api.Instance{
				Package:  "a/b/c",
				Instance: instRef,
			})
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})
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
