// Copyright 2023 The LUCI Authors.
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

package execmockserver

import (
	"encoding/gob"
	"os"
	"reflect"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func split2(s string, _ uint64) (string, string) {
	toks := strings.SplitN(s, "=", 2)
	if len(toks) != 2 {
		panic(errors.Fmt("splitting by = yielded != 2 tokens: %q", toks))
	}
	return toks[0], toks[1]
}

type TestStruct struct {
	Things []string
}

type List struct {
	V string
	P *List
}

func init() {
	gob.Register(TestStruct{})
	gob.Register(List{})
}

func TestServer(t *testing.T) {
	setenv := func(key, value string) {
		if err := os.Setenv(key, value); err != nil {
			t.Fatal(err)
		}
	}

	ftt.Run(`TestServer`, t, func(t *ftt.Test) {
		srv := Start()
		defer srv.Close()

		t.Run(`simple types`, func(t *ftt.Test) {
			input := InvocationInput{
				RunnerID:    1,
				RunnerInput: "world",
			}
			capturedOutput := ""
			serverCb := func(output string, stk string, err error) {
				capturedOutput = output
			}
			setenv(split2(RegisterInvocation(srv, &input, serverCb)))
			exitcode, intercepted := ClientIntercept(map[uint64]reflect.Value{1: reflect.ValueOf(func(input string) (string, int, error) {
				return "hello " + input, 123, nil
			})})

			assert.Loosely(t, intercepted, should.BeTrue)
			assert.Loosely(t, exitcode, should.Equal(123))
			assert.Loosely(t, capturedOutput, should.Equal("hello world"))
		})

		t.Run(`structs`, func(t *ftt.Test) {
			input := InvocationInput{
				RunnerID:    1,
				RunnerInput: TestStruct{[]string{"hello"}},
			}
			var capturedOutput TestStruct
			serverCb := func(output TestStruct, stk string, err error) {
				capturedOutput = output
			}
			setenv(split2(RegisterInvocation(srv, &input, serverCb)))
			exitcode, intercepted := ClientIntercept(map[uint64]reflect.Value{1: reflect.ValueOf(func(input TestStruct) (TestStruct, int, error) {
				return TestStruct{append(input.Things, "world")}, 123, nil
			})})

			assert.Loosely(t, intercepted, should.BeTrue)
			assert.Loosely(t, exitcode, should.Equal(123))
			assert.Loosely(t, capturedOutput, should.Match(TestStruct{[]string{"hello", "world"}}))
		})

		t.Run(`*structs`, func(t *ftt.Test) {
			input := InvocationInput{
				RunnerID:    1,
				RunnerInput: &TestStruct{[]string{"hello"}},
			}
			var capturedOutput *TestStruct
			serverCb := func(output *TestStruct, stk string, err error) {
				capturedOutput = output
			}
			setenv(split2(RegisterInvocation(srv, &input, serverCb)))
			exitcode, intercepted := ClientIntercept(map[uint64]reflect.Value{1: reflect.ValueOf(func(input *TestStruct) (*TestStruct, int, error) {
				return &TestStruct{append(input.Things, "world")}, 123, nil
			})})

			assert.Loosely(t, intercepted, should.BeTrue)
			assert.Loosely(t, exitcode, should.Equal(123))
			assert.Loosely(t, capturedOutput, should.Match(&TestStruct{[]string{"hello", "world"}}))
		})

		t.Run(`*nested structs`, func(t *ftt.Test) {
			input := InvocationInput{
				RunnerID:    1,
				RunnerInput: &List{"hello", &List{"there", nil}},
			}
			var capturedOutput *List
			serverCb := func(output *List, stk string, err error) {
				capturedOutput = output
			}
			setenv(split2(RegisterInvocation(srv, &input, serverCb)))
			exitcode, intercepted := ClientIntercept(map[uint64]reflect.Value{1: reflect.ValueOf(func(input *List) (*List, int, error) {
				return &List{"stuff", input}, 123, nil
			})})

			assert.Loosely(t, intercepted, should.BeTrue)
			assert.Loosely(t, exitcode, should.Equal(123))
			assert.Loosely(t, capturedOutput, should.Match(&List{"stuff", &List{"hello", &List{"there", nil}}}))
		})

		t.Run(`panic function (error)`, func(t *ftt.Test) {
			input := InvocationInput{
				RunnerID:    1,
				RunnerInput: "input",
			}
			var capturedOutput string
			var capturedErr error
			serverCb := func(output string, stk string, err error) {
				capturedOutput = output
				capturedErr = err
			}
			setenv(split2(RegisterInvocation(srv, &input, serverCb)))
			exitcode, intercepted := ClientIntercept(map[uint64]reflect.Value{1: reflect.ValueOf(func(input string) (string, int, error) {
				panic(errors.New("boom"))
			})})

			assert.Loosely(t, intercepted, should.BeTrue)
			assert.Loosely(t, exitcode, should.Equal(1))
			assert.Loosely(t, capturedOutput, should.BeBlank)
			assert.Loosely(t, capturedErr, should.ErrLike("boom"))
		})

		t.Run(`panic function (string)`, func(t *ftt.Test) {
			input := InvocationInput{
				RunnerID:    1,
				RunnerInput: "input",
			}
			var capturedOutput string
			var capturedErr error
			serverCb := func(output string, stk string, err error) {
				capturedOutput = output
				capturedErr = err
			}
			setenv(split2(RegisterInvocation(srv, &input, serverCb)))
			exitcode, intercepted := ClientIntercept(map[uint64]reflect.Value{1: reflect.ValueOf(func(input string) (string, int, error) {
				panic("boom")
			})})

			assert.Loosely(t, intercepted, should.BeTrue)
			assert.Loosely(t, exitcode, should.Equal(1))
			assert.Loosely(t, capturedOutput, should.BeBlank)
			assert.Loosely(t, capturedErr, should.ErrLike("boom"))
		})
	})
}
