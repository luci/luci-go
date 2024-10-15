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

func TestContainSubstring(t *testing.T) {
	t.Parallel()

	t.Run("pass", shouldPass(ContainSubstring("lo")("hello")))
	t.Run("fail", shouldFail(ContainSubstring("lo")("nerp")))
}

func TestNotContainSubstring(t *testing.T) {
	t.Parallel()

	t.Run("pass", shouldPass(NotContainSubstring("lo")("nerp")))
	t.Run("fail", shouldFail(NotContainSubstring("lo")("hello")))
}

func TestHaveSuffix(t *testing.T) {
	t.Parallel()

	t.Run("pass", shouldPass(HaveSuffix(".nerp")("hello.nerp")))
	t.Run("fail", shouldFail(HaveSuffix(".nerp")("hello.exe")))
}

func TestNotHaveSuffix(t *testing.T) {
	t.Parallel()

	t.Run("pass", shouldPass(NotHaveSuffix(".nerp")("hello.exe")))
	t.Run("fail", shouldFail(NotHaveSuffix(".nerp")("hello.nerp")))
}

func TestHavePrefix(t *testing.T) {
	t.Parallel()

	t.Run("pass", shouldPass(HavePrefix("nerp.")("nerp.hello")))
	t.Run("fail", shouldFail(HavePrefix("nerp.")("exe.hello")))
}

func TestNotHavePrefix(t *testing.T) {
	t.Parallel()

	t.Run("pass", shouldPass(NotHavePrefix("nerp.")("exe.hello")))
	t.Run("fail", shouldFail(NotHavePrefix("nerp.")("nerp.hello")))
}

func TestBeBlank(t *testing.T) {
	t.Parallel()

	t.Run("pass1", shouldPass(BeBlank("")))
	t.Run("pass2", shouldPass(BeBlank("  ")))
	t.Run("pass3", shouldPass(BeBlank("\t")))
	t.Run("fail1", shouldFail(BeBlank("a")))
	t.Run("fail2", shouldFail(BeBlank("a ")))
	t.Run("fail3", shouldFail(BeBlank("4")))
}

func TestNotBeBlank(t *testing.T) {
	t.Parallel()

	t.Run("fail1", shouldFail(NotBeBlank("")))
	t.Run("fail2", shouldFail(NotBeBlank("  ")))
	t.Run("fail3", shouldFail(NotBeBlank("\t")))
	t.Run("pass1", shouldPass(NotBeBlank("a")))
	t.Run("pass2", shouldPass(NotBeBlank("a ")))
	t.Run("pass3", shouldPass(NotBeBlank("4")))
}

func TestMatchRegexp(t *testing.T) {
	t.Parallel()

	t.Run("fail1", shouldFail(MatchRegexp("h.llo")("goodbye"), "did not match"))
	t.Run("fail2", shouldFail(MatchRegexp("bad(")("goodbye"), "missing closing )"))

	t.Run("pass1", shouldPass(MatchRegexp("h.llo")("hello")))
}
