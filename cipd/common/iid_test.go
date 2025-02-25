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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
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
		ftt.Run(fmt.Sprintf("Works with %q", iid), t, func(t *ftt.Test) {
			assert.Loosely(t, ValidateInstanceID(iid, KnownHash), should.BeNil)
			assert.Loosely(t, ValidateInstanceID(iid, AnyHash), should.BeNil)
		})
	}

	bad := []string{
		"",
		"€aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"gaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"AAAaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajM==", // no padding allowed
		"/7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUC", // should be URL encoding, NOT std
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUA", // unspecified hash algo
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAC",                 // bad digest len for an algo
	}
	for _, iid := range bad {
		ftt.Run(fmt.Sprintf("Fails with %q", iid), t, func(t *ftt.Test) {
			assert.Loosely(t, ValidateInstanceID(iid, KnownHash), should.NotBeNil)
			assert.Loosely(t, ValidateInstanceID(iid, AnyHash), should.NotBeNil)
		})
	}

	unknown := []string{
		"B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUD", // unrecognized hash algo
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",      // happens to be looking like unknown hash algo
	}
	for _, iid := range unknown {
		ftt.Run(fmt.Sprintf("Works with %q", iid), t, func(t *ftt.Test) {
			assert.Loosely(t, ValidateInstanceID(iid, KnownHash), should.NotBeNil)
			assert.Loosely(t, ValidateInstanceID(iid, AnyHash), should.BeNil)
		})
	}
}

func TestValidateObjectRef(t *testing.T) {
	t.Parallel()

	ftt.Run("SHA1", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "0123456789abcdef0123456789abcdef00000000",
		}, KnownHash), should.BeNil)

		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: "abcd",
		}, KnownHash), should.ErrLike("expecting 40 chars, got 4"))

		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: strings.Repeat("A", 40), // uppercase are forbidden
		}, KnownHash), should.ErrLike("wrong char"))
	})

	ftt.Run("SHA256", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447",
		}, KnownHash), should.BeNil)

		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: "abcd",
		}, KnownHash), should.ErrLike("expecting 64 chars, got 4"))

		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("A", 64), // uppercase are forbidden
		}, KnownHash), should.ErrLike("wrong char"))
	})

	ftt.Run("Some future hash in KnownHash mode", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  33,
			HexDigest: "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447",
		}, KnownHash), should.ErrLike("unsupported unknown hash algorithm #33"))
	})

	ftt.Run("Some future hash in AnyHash mode", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  33,
			HexDigest: "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447",
		}, AnyHash), should.BeNil)

		// Still checks that the hex digest looks like a digest.
		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  33,
			HexDigest: "abc",
		}, KnownHash), should.ErrLike("uneven number of symbols"))

		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{
			HashAlgo:  33,
			HexDigest: strings.Repeat("A", 64), // uppercase are forbidden
		}, KnownHash), should.ErrLike("wrong char"))
	})

	ftt.Run("Bad args", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateObjectRef(nil, AnyHash), should.ErrLike("not provided"))
		assert.Loosely(t, ValidateObjectRef(&api.ObjectRef{HashAlgo: 0}, AnyHash), should.ErrLike("unspecified hash algo"))
	})
}

func TestRefIIDConversion(t *testing.T) {
	t.Parallel()

	ftt.Run("SHA1 works", t, func(t *ftt.Test) {
		sha1hex := strings.Repeat("a", 40)
		sha1iid := sha1hex // iid and hex digest coincide for SHA1

		assert.Loosely(t, ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: sha1hex,
		}), should.Equal(sha1iid))

		assert.Loosely(t, InstanceIDToObjectRef(sha1iid), should.Match(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: sha1hex,
		}))
	})

	ftt.Run("SHA256 works", t, func(t *ftt.Test) {
		sha256hex := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
		sha256iid := "qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-4XSmaGSpEcC"

		assert.Loosely(t, ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: sha256hex,
		}), should.Equal(sha256iid))

		assert.Loosely(t, InstanceIDToObjectRef(sha256iid), should.Match(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: sha256hex,
		}))
	})

	ftt.Run("Some future unknown hash", t, func(t *ftt.Test) {
		hex := strings.Repeat("a", 60)
		iid := "qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqIQ"

		assert.Loosely(t, ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  33,
			HexDigest: hex,
		}), should.Equal(iid))

		assert.Loosely(t, InstanceIDToObjectRef(iid), should.Match(&api.ObjectRef{
			HashAlgo:  33,
			HexDigest: hex,
		}))
	})

	ftt.Run("Wrong length in InstanceIDToObjectRef", t, func(t *ftt.Test) {
		assert.Loosely(t, func() {
			InstanceIDToObjectRef("aaaa")
		}, should.PanicLike("not a valid size for an encoded digest"))
	})

	ftt.Run("Bad format in InstanceIDToObjectRef", t, func(t *ftt.Test) {
		assert.Loosely(t, func() {
			InstanceIDToObjectRef("qUiQTy8PR5uPgZdpSzAYSw0u0cHNKh7A-?XSmaGSpEcC")
		}, should.PanicLike("illegal base64 data"))
	})
}
