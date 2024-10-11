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

package main

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPathValidation(t *testing.T) {
	ftt.Run(`Path too short.`, t, func(t *ftt.Test) {
		err := validateCryptoKeysKMSPath("bad/path")
		assert.Loosely(t, err, should.ErrLike("path should have the form"))
	})
	ftt.Run(`Path too long.`, t, func(t *ftt.Test) {
		err := validateCryptoKeysKMSPath("bad/long/long/long/long/long/long/long/long/long/path")
		assert.Loosely(t, err, should.ErrLike("path should have the form"))
	})
	ftt.Run(`Path misspelling.`, t, func(t *ftt.Test) {
		err := validateCryptoKeysKMSPath("projects/chromium/oops/global/keyRings/test/cryptoKeys/my_key")
		assert.Loosely(t, err, should.ErrLike("expected component 3"))
	})
	ftt.Run(`Good path.`, t, func(t *ftt.Test) {
		err := validateCryptoKeysKMSPath("projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key")
		assert.Loosely(t, err, should.BeNil)
	})
	ftt.Run(`Good path.`, t, func(t *ftt.Test) {
		err := validateCryptoKeysKMSPath("projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key/cryptoKeyVersions/1")
		assert.Loosely(t, err, should.BeNil)
	})
	ftt.Run(`Support leading slash.`, t, func(t *ftt.Test) {
		err := validateCryptoKeysKMSPath("/projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key")
		assert.Loosely(t, err, should.BeNil)
	})
}

func TestCryptoRunParse(t *testing.T) {
	ctx := context.Background()
	path := "/projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key"
	flags := []string{"-input", "hello", "-output", "goodbye"}
	testParse := func(flags, args []string) error {
		c := cryptRun{}
		c.Init(auth.Options{})
		c.GetFlags().Parse(flags)
		return c.Parse(ctx, args)
	}
	ftt.Run(`Make sure that Parse fails with no positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{})
		assert.Loosely(t, err, should.ErrLike("positional arguments missing"))
	})
	ftt.Run(`Make sure that Parse fails with too many positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"one", "two"})
		assert.Loosely(t, err, should.ErrLike("unexpected positional arguments"))
	})
	ftt.Run(`Make sure that Parse fails with no input.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-output", "goodbye"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("input file"))
	})
	ftt.Run(`Make sure that Parse fails with no output.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-input", "hello"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("output location"))
	})
	ftt.Run(`Make sure that Parse fails with bad key path.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"abcdefg"})
		assert.Loosely(t, err, should.NotBeNil)
	})
	ftt.Run(`Make sure that Parse works with everything set right.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{path})
		assert.Loosely(t, err, should.BeNil)
	})
}

func TestVerifyRunParse(t *testing.T) {
	ctx := context.Background()
	path := "/projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key/cryptoKeyVersions/1"
	flags := []string{"-input", "hello", "-input-sig", "goodbye"}
	testParse := func(flags, args []string) error {
		v := verifyRun{}
		v.Init(auth.Options{})
		v.GetFlags().Parse(flags)
		return v.Parse(ctx, args)
	}
	ftt.Run(`Make sure that Parse fails with no positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{})
		assert.Loosely(t, err, should.ErrLike("positional arguments missing"))
	})
	ftt.Run(`Make sure that Parse fails with too many positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"one", "two"})
		assert.Loosely(t, err, should.ErrLike("unexpected positional arguments"))
	})
	ftt.Run(`Make sure that Parse fails with no input.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-input-sig", "goodbye"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("input file"))
	})
	ftt.Run(`Make sure that Parse fails with no input sig.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-input", "hello"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("input sig"))
	})
	ftt.Run(`Make sure that Parse fails with bad key path.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"abcdefg"})
		assert.Loosely(t, err, should.NotBeNil)
	})
	ftt.Run(`Make sure that Parse works with everything set right.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{path})
		assert.Loosely(t, err, should.BeNil)
	})
}

func TestSignRunParse(t *testing.T) {
	ctx := context.Background()
	path := "/projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key/cryptoKeyVersions/1"
	flags := []string{"-input", "hello", "-output", "goodbye"}
	testParse := func(flags, args []string) error {
		s := signRun{}
		s.Init(auth.Options{})
		s.GetFlags().Parse(flags)
		return s.Parse(ctx, args)
	}
	ftt.Run(`Make sure that Parse fails with no positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{})
		assert.Loosely(t, err, should.ErrLike("positional arguments missing"))
	})
	ftt.Run(`Make sure that Parse fails with too many positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"one", "two"})
		assert.Loosely(t, err, should.ErrLike("unexpected positional arguments"))
	})
	ftt.Run(`Make sure that Parse fails with no input.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-output", "goodbye"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("input file"))
	})
	ftt.Run(`Make sure that Parse fails with no input sig.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-input", "hello"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("output location"))
	})
	ftt.Run(`Make sure that Parse fails with bad key path.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"abcdefg"})
		assert.Loosely(t, err, should.NotBeNil)
	})
	ftt.Run(`Make sure that Parse works with everything set right.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{path})
		assert.Loosely(t, err, should.BeNil)
	})
}

func TestDownloadRunParse(t *testing.T) {
	ctx := context.Background()
	path := "/projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key/cryptoKeyVersions/1"
	flags := []string{"-output", "goodbye"}
	testParse := func(flags, args []string) error {
		d := downloadRun{}
		d.Init(auth.Options{})
		d.GetFlags().Parse(flags)
		return d.Parse(ctx, args)
	}
	ftt.Run(`Make sure that Parse fails with no positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{})
		assert.Loosely(t, err, should.ErrLike("positional arguments missing"))
	})
	ftt.Run(`Make sure that Parse fails with too many positional args.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"one", "two"})
		assert.Loosely(t, err, should.ErrLike("unexpected positional arguments"))
	})
	ftt.Run(`Make sure that Parse fails with input.`, t, func(t *ftt.Test) {
		err := testParse([]string{"-input", "hello", "-output", "goodbye"}, []string{path})
		assert.Loosely(t, err, should.ErrLike("output location"))
	})
	ftt.Run(`Make sure that Parse fails with bad key path.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{"abcdefg"})
		assert.Loosely(t, err, should.NotBeNil)
	})
	ftt.Run(`Make sure that Parse works with everything set right.`, t, func(t *ftt.Test) {
		err := testParse(flags, []string{path})
		assert.Loosely(t, err, should.BeNil)
	})
}
