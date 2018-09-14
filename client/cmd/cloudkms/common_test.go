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
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/auth"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPathValidation(t *testing.T) {
	Convey(`Path too short.`, t, func() {
		err := validateCryptoKeysKMSPath("bad/path")
		So(err, ShouldErrLike, "path should have the form")
	})
	Convey(`Path too long.`, t, func() {
		err := validateCryptoKeysKMSPath("bad/long/long/long/long/long/long/long/long/long/path")
		So(err, ShouldErrLike, "path should have the form")
	})
	Convey(`Path misspelling.`, t, func() {
		err := validateCryptoKeysKMSPath("projects/chromium/oops/global/keyRings/test/cryptoKeys/my_key")
		So(err, ShouldErrLike, "expected component 3")
	})
	Convey(`Good path.`, t, func() {
		err := validateCryptoKeysKMSPath("projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key")
		So(err, ShouldBeNil)
	})
	Convey(`Support leading slash.`, t, func() {
		err := validateCryptoKeysKMSPath("/projects/chromium/locations/global/keyRings/test/cryptoKeys/my_key")
		So(err, ShouldBeNil)
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
	Convey(`Make sure that Parse fails with no positional args.`, t, func() {
		err := testParse(flags, []string{})
		So(err, ShouldErrLike, "positional arguments missing")
	})
	Convey(`Make sure that Parse fails with too many positional args.`, t, func() {
		err := testParse(flags, []string{"one", "two"})
		So(err, ShouldErrLike, "unexpected positional arguments")
	})
	Convey(`Make sure that Parse fails with no input.`, t, func() {
		err := testParse([]string{"-output", "goodbye"}, []string{path})
		So(err, ShouldErrLike, "input file")
	})
	Convey(`Make sure that Parse fails with no output.`, t, func() {
		err := testParse([]string{"-input", "hello"}, []string{path})
		So(err, ShouldErrLike, "output location")
	})
	Convey(`Make sure that Parse fails with bad key path.`, t, func() {
		err := testParse(flags, []string{"abcdefg"})
		So(err, ShouldNotBeNil)
	})
	Convey(`Make sure that Parse works with everything set right.`, t, func() {
		err := testParse(flags, []string{path})
		So(err, ShouldBeNil)
	})
}
