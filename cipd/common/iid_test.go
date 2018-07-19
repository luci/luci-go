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

package common

import (
	"fmt"
	"strings"
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateInstanceID(t *testing.T) {
	t.Parallel()

	good := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0123456789abcdefaaaaaaaaaaaaaaaaaaaaaaaa",
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUC",
		"-dXsPJ3XDdzO3GLNqekTAmNfIXtE697vame6_4_HNUkC",
		"ytsp2xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C",
	}
	for _, iid := range good {
		Convey(fmt.Sprintf("Works with %q", iid), t, func() {
			So(ValidateInstanceID(iid), ShouldBeNil)
		})
	}

	bad := []string{
		"",
		"€aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"gaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"AAAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajM==", // no padding allowed
		"/7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUC", // should be URL encoding, NOT std
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUA", // unspecified hash algo
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUD", // unrecognized hash algo
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAC",                 // bad digest len for an algo
	}
	for _, iid := range bad {
		Convey(fmt.Sprintf("Fails with %q", iid), t, func() {
			So(ValidateInstanceID(iid), ShouldNotBeNil)
		})
	}
}

func TestValidateObjectRef(t *testing.T) {
	t.Parallel()

	Convey("SHA1", t, func() {
		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "0123456789abcdef0123456789abcdef00000000",
		}), ShouldBeNil)

		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "abc",
		}), ShouldErrLike, "expecting 40 chars, got 3")

		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: strings.Repeat("A", 40), // uppercase are forbidden
		}), ShouldErrLike, "wrong char")
	})

	Convey("SHA256", t, func() {
		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447",
		}), ShouldBeNil)

		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: "abc",
		}), ShouldErrLike, "expecting 64 chars, got 3")

		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("A", 64), // uppercase are forbidden
		}), ShouldErrLike, "wrong char")
	})

	Convey("Bad args", t, func() {
		So(ValidateObjectRef(nil), ShouldErrLike, "not provided")
		So(ValidateObjectRef(&api.ObjectRef{HashAlgo: 12345}), ShouldErrLike, "unsupported")
	})
}

func TestRefIIDConversion(t *testing.T) {
	t.Parallel()

	Convey("SHA1 works", t, func() {
		sha1hex := strings.Repeat("a", 40)
		sha1iid := sha1hex // iid and hex digest coincide for SHA1

		So(ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: sha1hex,
		}), ShouldEqual, sha1iid)

		So(InstanceIDToObjectRef(sha1iid), ShouldResemble, &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: sha1hex,
		})
	})

	Convey("SHA256 works", t, func() {
		sha256hex := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
		sha256iid := "qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-4XSmaGSpEcC"

		So(ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: sha256hex,
		}), ShouldEqual, sha256iid)

		So(InstanceIDToObjectRef(sha256iid), ShouldResemble, &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: sha256hex,
		})
	})

	Convey("Wrong length in InstanceIDToObjectRef", t, func() {
		So(func() {
			InstanceIDToObjectRef("aaaa")
		}, ShouldPanicLike, "not a valid size for a digest")
	})

	Convey("Bad format in InstanceIDToObjectRef", t, func() {
		So(func() {
			InstanceIDToObjectRef("qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-?XSmaGSpEcC")
		}, ShouldPanicLike, "illegal base64 data")
	})
}
