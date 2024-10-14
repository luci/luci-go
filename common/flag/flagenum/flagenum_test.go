// Copyright 2016 The LUCI Authors.
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

package flagenum

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testValue string

const (
	testFoo    testValue = "test-foo"
	testBar    testValue = "test-bar"
	testEmpty  testValue = "test-empty"
	testQuoted testValue = `test-'"`
)

var (
	testEnum = Enum{
		"foo":      testFoo,
		"bar":      testBar,
		"":         testEmpty,
		`'"`:       testQuoted,
		"failboat": 123,
	}
)

func (tv *testValue) Set(v string) error {
	return testEnum.FlagSet(tv, v)
}

func (tv *testValue) String() string {
	return testEnum.FlagString(tv)
}

func (tv *testValue) UnmarshalJSON(data []byte) error {
	return testEnum.JSONUnmarshal(tv, data)
}

func (tv testValue) MarshalJSON() ([]byte, error) {
	return testEnum.JSONMarshal(tv)
}

type testStruct struct {
	Name string
	A    string
}

var (
	testStructFoo    = testStruct{"testStructFoo", "FOO"}
	testStructBar    = testStruct{"testStructBar", "BAR"}
	testStructQuotes = testStruct{"testStructQuotes", "QUOTES"}

	testStructEnum = Enum{
		"foo": testStructFoo,
		"bar": testStructBar,
		`'"`:  testStructQuotes,
	}
)

func (tv *testStruct) Set(v string) error {
	return testStructEnum.FlagSet(tv, v)
}

func (tv *testStruct) String() string {
	return testStructEnum.FlagString(tv)
}

func (tv *testStruct) UnmarshalJSON(data []byte) error {
	return testStructEnum.JSONUnmarshal(tv, data)
}

func (tv testStruct) MarshalJSON() ([]byte, error) {
	return testStructEnum.JSONMarshal(tv)
}

func TestEnum(t *testing.T) {
	ftt.Run(`A testing enumeration`, t, func(t *ftt.Test) {

		t.Run(`String for "testFoo" is "foo".`, func(t *ftt.Test) {
			assert.Loosely(t, testEnum.GetKey(testFoo), should.Equal("foo"))
		})

		t.Run(`String for "testQuoted" is ['"].`, func(t *ftt.Test) {
			assert.Loosely(t, testEnum.GetKey(testQuoted), should.Equal(`'"`))
		})

		t.Run(`String for unregistered type is "".`, func(t *ftt.Test) {
			assert.Loosely(t, testEnum.GetKey(0x10), should.BeEmpty)
		})

		t.Run(`Lists choices as: "", '", bar, failboat, foo.`, func(t *ftt.Test) {
			assert.Loosely(t, testEnum.Choices(), should.Equal(`"", '", bar, failboat, foo`))
		})
	})
}

func TestStringEnum(t *testing.T) {
	ftt.Run(`A testing enumeration for a string type`, t, func(t *ftt.Test) {
		t.Run(`Panics when deserialized from an incompatible type.`, func(t *ftt.Test) {
			var value testValue
			assert.Loosely(t, func() { testEnum.setValue(&value, "failboat") }, should.Panic)
		})

		t.Run(`Panics when attempting to set an unsettable value.`, func(t *ftt.Test) {
			assert.Loosely(t, func() { testEnum.setValue(123, "foo") }, should.Panic)
		})

		t.Run(`Binds to an flag field.`, func(t *ftt.Test) {
			var value testValue

			t.Run(`When used as a flag`, func(t *ftt.Test) {
				fs := flag.NewFlagSet("test", flag.ContinueOnError)
				fs.Var(&value, "test", "Test flag.")

				t.Run(`Sets "value" when set.`, func(t *ftt.Test) {
					err := fs.Parse([]string{"-test", "bar"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, value, should.Equal(testBar))
				})

				t.Run(`Rejects an invalid enum mapping.`, func(t *ftt.Test) {
					err := fs.Parse([]string{"-test", "baz"})
					assert.Loosely(t, err, should.NotBeNil)
				})
			})

			t.Run(`When marshalling to JSON`, func(t *ftt.Test) {
				var s struct {
					Value testValue `json:"value"`
				}

				t.Run(`Marshals to JSON as its enumeration value.`, func(t *ftt.Test) {
					s.Value = testFoo
					data, err := json.Marshal(&s)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, string(data), should.Equal(`{"value":"foo"}`))

					t.Run(`And unmarshals to its Value.`, func(t *ftt.Test) {
						err := json.Unmarshal(data, &s)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, s.Value, should.Equal(testFoo))
					})
				})

				t.Run(`Unmarshals {"value":"bar"} to "testBar".`, func(t *ftt.Test) {
					err := json.Unmarshal([]byte(`{"value":"bar"}`), &s)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.Value, should.Equal(testBar))
				})

				t.Run(`Fails to Unmarshal invalid JSON.`, func(t *ftt.Test) {
					err := json.Unmarshal([]byte(`{"value":123}`), &s)
					assert.Loosely(t, err, should.NotBeNil)
				})
			})
		})
	})
}

func TestStructEnum(t *testing.T) {
	ftt.Run(`A testing enumeration for a struct type`, t, func(t *ftt.Test) {

		t.Run(`Panics when attempting to set an unsettable value.`, func(t *ftt.Test) {
			assert.Loosely(t, func() { testStructEnum.setValue(123, "foo") }, should.Panic)
		})

		t.Run(`Binds to an flag field.`, func(t *ftt.Test) {
			var value testStruct

			testCases := []struct {
				V string
				S testStruct
			}{
				{"foo", testStructFoo},
				{"bar", testStructBar},
				{`'"`, testStructQuotes},
			}

			t.Run(`When used as a flag`, func(t *ftt.Test) {
				fs := flag.NewFlagSet("test", flag.ContinueOnError)
				fs.Var(&value, "test", "Test flag.")

				for _, tc := range testCases {
					t.Run(fmt.Sprintf(`Sets "value" to %s when set to [%s].`, tc.S.Name, tc.V), func(t *ftt.Test) {
						err := fs.Parse([]string{"-test", tc.V})
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, value, should.Resemble(tc.S))
					})
				}

				t.Run(`Rejects an invalid enum mapping.`, func(t *ftt.Test) {
					err := fs.Parse([]string{"-test", "baz"})
					assert.Loosely(t, err, should.NotBeNil)
				})
			})

			t.Run(`When marshalling to JSON`, func(t *ftt.Test) {
				var s struct {
					Value testStruct `json:"value"`
				}

				for _, tc := range testCases {
					t.Run(fmt.Sprintf(`Marshals %s to JSON as its enumeration value, [%s].`,
						tc.S.Name, tc.V), func(t *ftt.Test) {
						s.Value = tc.S
						data, err := json.Marshal(&s)
						assert.Loosely(t, err, should.BeNil)

						mkey, _ := json.Marshal(&tc.V)
						assert.Loosely(t, string(data), should.Equal(fmt.Sprintf(`{"value":%s}`, string(mkey))))

						t.Run(`And unmarshals to its Value.`, func(t *ftt.Test) {
							err := json.Unmarshal(data, &s)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, s.Value, should.Resemble(tc.S))
						})
					})
				}

				t.Run(`Unmarshals {"value":"bar"} to "testStructBar".`, func(t *ftt.Test) {
					err := json.Unmarshal([]byte(`{"value":"bar"}`), &s)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.Value, should.Resemble(testStructBar))
				})

				t.Run(`Fails to Unmarshal invalid JSON.`, func(t *ftt.Test) {
					err := json.Unmarshal([]byte(`{"value":123}`), &s)
					assert.Loosely(t, err, should.NotBeNil)
				})
			})
		})
	})
}
