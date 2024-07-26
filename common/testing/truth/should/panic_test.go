// Copyright 2024 The LUCI Authors.
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

package should

import (
	"errors"
	"fmt"
	"testing"
)

var errGoldenError error = errors.New("this is the one true error")

func TestPanicHappyPath(t *testing.T) {
	shouldPass(Panic(func() {
		panic("hi")
	}))(t)
}

func TestPanicSadPath(t *testing.T) {
	shouldFail(Panic(func() {
		// do nothing
	}))(t)
}

func TestNotPanicHappyPath(t *testing.T) {
	shouldPass(NotPanic(func() {
		// do nothing
	}))(t)
}

func TestNotPanicSadPath(t *testing.T) {
	shouldFail(NotPanic(func() {
		panic("hi")
	}))(t)
}
func TestPanicLikeHappyPath(t *testing.T) {
	shouldPass(PanicLike("one true error")(func() {
		panic(errGoldenError)
	}))(t)
}

func TestPanicLikeSadPath(t *testing.T) {
	shouldFail(PanicLike("one true error")(func() {
		// don't panic!
	}))(t)
}

func TestPanicWrongException(t *testing.T) {
	shouldFail(PanicLike("one true error")(func() {
		panic(errors.New("wrong"))
	}))(t)
}

func TestPanicWrappedException(t *testing.T) {
	shouldPass(PanicLike(errGoldenError)(func() {
		panic(fmt.Errorf("wrapped: %w", errGoldenError))
	}))(t)
}

func TestPanicWrongTypeThrown(t *testing.T) {
	shouldFail(PanicLike("one true error")(func() {
		panic(72)
	}))(t)
}

func TestPanicErrorDoesNotMatch(t *testing.T) {
	shouldFail(PanicLike(errGoldenError)(func() {
		panic(errors.New("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	}))(t)
}
