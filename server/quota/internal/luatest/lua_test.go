// Copyright 2022 The LUCI Authors.
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

package luatest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	luajson "github.com/alicebob/gopher-json"
	lua "github.com/yuin/gopher-lua"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var hexFinder = regexp.MustCompile(`\\x[a-f0-9]{2}`)

// hackedFormat is like the built in implementation of string.format, except
// that it correctly implements %q on strings containing binary characters.
func hackedFormat(L *lua.LState) int {
	str := L.CheckString(1)
	args := make([]any, L.GetTop()-1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		args[i-2] = L.Get(i)
	}
	npat := strings.Count(str, "%") - strings.Count(str, "%%")

	ret := hexFinder.ReplaceAllStringFunc(fmt.Sprintf(str, args[:intMin(npat, len(args))]...), func(s string) string {
		dec, err := strconv.ParseUint(s[2:], 16, 8)
		if err != nil {
			panic(err)
		}
		return fmt.Sprintf("\\%03d", dec)
	})

	L.Push(lua.LString(ret))
	return 1
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func TestLua(t *testing.T) {
	// gopher-lua doesn't implement os.date correctly, so having this unset
	// results in a crash because luaunt passes the result of os.getenv as the
	// first argument to `os.date` which doesn't like `args.n == 1` with the arg
	// value being nil. So, we set it to the real default explicitly.
	check(os.Setenv("LUAUNIT_DATEFMT", "%c"))
	defer func() {
		check(os.Unsetenv("LUAUNIT_DATEFMT"))
	}()
	oldLuaPath := os.Getenv("LUA_PATH")
	if os.Unsetenv("LUA_PATH") == nil {
		defer func() {
			check(os.Setenv("LUA_PATH", oldLuaPath))
		}()
	}
	check(os.Chdir("testdata"))
	defer func() {
		check(os.Chdir(".."))
	}()

	matches, err := filepath.Glob("*_test.lua")
	if err != nil {
		t.Fatalf("could not glob: %s", err)
	}
	if len(matches) == 0 {
		t.Fatal("found no lua tests")
	}

	ftt.Run(`lua`, t, func(t *ftt.Test) {
		for _, filename := range matches {
			t.Run(filename, func(c *ftt.Test) {
				L := lua.NewState(lua.Options{})
				defer L.Close()

				// install `cjson` global; used for DUMP debugging function.
				luajson.Loader(L)
				mod := L.Get(1)
				L.Pop(1)
				L.SetGlobal("cjson", mod)

				L.GetGlobal("string").(*lua.LTable).RawSet(lua.LString("format"), L.NewFunction(hackedFormat))
				assert.Loosely(c, L.DoFile(filename), should.BeNil)
				assert.Loosely(c, L.CheckInt(1), should.BeZero, truth.Explain("luaunit tests failed"))
				t.Log() // space between luaunit outputs
				assert.Loosely(c, err, should.BeNil)
			})
		}
	})
}
