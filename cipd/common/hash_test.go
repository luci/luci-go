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

package common

import (
	"strings"
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNewHash(t *testing.T) {
	t.Parallel()

	Convey("Unspecified", t, func() {
		_, err := NewHash(api.HashAlgo_HASH_ALGO_UNSPECIFIED)
		So(err, ShouldErrLike, "not specified")
	})

	Convey("Unknown", t, func() {
		_, err := NewHash(12345)
		So(err, ShouldErrLike, "unsupported")
	})

	Convey("SHA1", t, func() {
		algo, err := NewHash(api.HashAlgo_SHA1)
		So(err, ShouldBeNil)
		So(algo, ShouldNotBeNil)
	})
}

func TestValidateObjectRef(t *testing.T) {
	t.Parallel()

	Convey("good", t, func() {
		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "0123456789abcdef0123456789abcdef00000000",
		}), ShouldBeNil)
	})

	Convey("bad", t, func() {
		So(ValidateObjectRef(nil), ShouldErrLike, "not provided")
		So(ValidateObjectRef(&api.ObjectRef{HashAlgo: 12345}), ShouldErrLike, "unsupported")
		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "abc",
		}), ShouldErrLike, "expecting 40 chars, got 3")
		So(ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: strings.Repeat("A", 40), // uppercase are forbidden
		}), ShouldErrLike, "wrong char")
	})
}
