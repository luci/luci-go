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
	"strings"
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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
		sha256iid := "qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-4XSmaGSpEc"

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
		}, ShouldPanicLike, "wrong length")
	})

	Convey("Bad format in InstanceIDToObjectRef", t, func() {
		So(func() {
			InstanceIDToObjectRef("qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-4XSmaG????")
		}, ShouldPanicLike, "illegal base64 data")
	})
}
