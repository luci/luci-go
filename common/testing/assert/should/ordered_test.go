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
	"testing"
)

func TestBeBetween(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeBetween(1, 100)(10)))

	t.Run("string", shouldPass(BeBetween("aaa", "zzzzz")("hello")))
	t.Run("string == lower", shouldFail(BeBetween("hello", "zzzzz")("hello"), "value was too low"))
	t.Run("string == upper", shouldFail(BeBetween("aaa", "hello")("hello"), "value was too high"))

	t.Run("lower > upper", shouldFail(BeBetween(100, 10)(20), "`lower` >= `upper`"))
	t.Run("lower == upper", shouldFail(BeBetween(10, 10)(20), "`lower` >= `upper`"))
}

func TestBeBetweenOrEqual(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeBetweenOrEqual(1, 100)(10)))

	t.Run("string", shouldPass(BeBetweenOrEqual("aaa", "zzzzz")("hello")))
	t.Run("string == lower", shouldPass(BeBetweenOrEqual("hello", "zzzzz")("hello")))
	t.Run("string == upper", shouldPass(BeBetweenOrEqual("aaa", "hello")("hello")))

	t.Run("lower == upper", shouldPass(BeBetweenOrEqual(10, 10)(10)))

	t.Run("string < lower", shouldFail(BeBetweenOrEqual("hello", "zzzzz")("aaa"), "value was too low"))
	t.Run("string > upper", shouldFail(BeBetweenOrEqual("aaa", "hello")("zzz"), "value was too high"))

	t.Run("lower > upper", shouldFail(BeBetweenOrEqual(100, 10)(20), "`lower` > `upper`"))
}

func TestBeLessThan(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeLessThan(100)(10)))
	t.Run("string", shouldPass(BeLessThan("hi")("aa")))

	t.Run("string == upper", shouldFail(BeLessThan("hi")("hi"), "too high"))
	t.Run("string > upper", shouldFail(BeLessThan("hi")("zz"), "too high"))
}

func TestBeLessThanOrEqual(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeLessThanOrEqual(100)(10)))
	t.Run("int equal", shouldPass(BeLessThanOrEqual(10)(10)))
	t.Run("string", shouldPass(BeLessThanOrEqual("hi")("aa")))
	t.Run("string equal", shouldPass(BeLessThanOrEqual("hi")("hi")))

	t.Run("string > upper", shouldFail(BeLessThanOrEqual("hi")("zz"), "too high"))
}

func TestBeGreaterThan(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeGreaterThan(10)(100)))
	t.Run("string", shouldPass(BeGreaterThan("aaa")("hello")))

	t.Run("string equal", shouldFail(BeGreaterThan("aaa")("aaa"), "too low"))
	t.Run("string smaller", shouldFail(BeGreaterThan("zz")("aaa"), "too low"))
}

func TestBeGreaterThanOrEqual(t *testing.T) {
	t.Parallel()

	t.Run("int", shouldPass(BeGreaterThanOrEqual(10)(100)))
	t.Run("string", shouldPass(BeGreaterThanOrEqual("aaaaa")("hi")))
	t.Run("string equal", shouldPass(BeGreaterThanOrEqual("aaaaa")("aaaaa")))

	t.Run("string smaller", shouldFail(BeGreaterThanOrEqual("zz")("hi"), "too low"))
}

func TestBeSorted(t *testing.T) {
	t.Parallel()

	t.Run("working", shouldPass(BeSorted([]string{"a", "b"})))
	t.Run("busted", shouldFail(BeSorted([]string{"z", "a", "b"})))
}
