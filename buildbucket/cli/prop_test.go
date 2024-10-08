// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"io/ioutil"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func mustStruct(data map[string]any) *structpb.Struct {
	ret, err := structpb.NewStruct(data)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestPropertiesFlag(t *testing.T) {
	t.Parallel()

	ftt.Run("PropertieFlag", t, func(t *ftt.Test) {
		props := &structpb.Struct{}
		f := PropertiesFlag(props)

		t.Run("File", func(t *ftt.Test) {
			file, err := ioutil.TempFile("", "")
			assert.Loosely(t, err, should.BeNil)
			defer file.Close()

			_, err = file.WriteString(`{
				"in-file-1": "orig",
				"in-file-2": "orig"
			}`)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, f.Set("@"+file.Name()), should.BeNil)

			assert.Loosely(t, props, should.Resemble(mustStruct(map[string]any{
				"in-file-1": "orig",
				"in-file-2": "orig",
			})))

			t.Run("Override", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("in-file-2=override"), should.BeNil)
				assert.Loosely(t, props, should.Resemble(mustStruct(map[string]any{
					"in-file-1": "orig",
					"in-file-2": "override",
				})))

				assert.Loosely(t, f.Set("a=b"), should.BeNil)
				assert.Loosely(t, props, should.Resemble(mustStruct(map[string]any{
					"in-file-1": "orig",
					"in-file-2": "override",
					"a":         "b",
				})))
			})
		})

		t.Run("Name=Value", func(t *ftt.Test) {
			assert.Loosely(t, f.Set("foo=bar"), should.BeNil)
			assert.Loosely(t, props, should.Resemble(mustStruct(map[string]any{
				"foo": "bar",
			})))

			t.Run("JSON", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("array=[1]"), should.BeNil)
				assert.Loosely(t, props, should.Resemble(mustStruct(map[string]any{
					"foo":   "bar",
					"array": []any{1},
				})))
			})

			t.Run("Trims spaces", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("array = [1]"), should.BeNil)
				assert.Loosely(t, props, should.Resemble(mustStruct(map[string]any{
					"foo":   "bar",
					"array": []any{1},
				})))
			})

			t.Run("Dup", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("foo=bar"), should.ErrLike(`duplicate property "foo`))
			})
		})
	})
}
