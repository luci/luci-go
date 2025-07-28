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

package templateproto

import (
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func parse(t testing.TB, template string) *File_Template {
	ret := &File_Template{}
	assert.Loosely(t, proto.UnmarshalText(template, ret), should.BeNil)
	return ret
}

func TestTemplateNormalize(t *testing.T) {
	t.Parallel()

	badCases := []struct {
		err, data string
	}{
		{"body is empty", ""},

		{"param \"\": invalid name", `body: "foofball" param: <key: "" value: <>>`},
		{"param \"$#foo\": malformed name", `body: "foofball" param: <key: "$#foo" value:<>>`},
		{"param \"${}\": malformed name", `body: "foofball" param: <key: "${}" value:<>>`},
		{"param \"blah${cool}nerd\": malformed name", `body: "foofball" param: <key: "blah${cool}nerd" value:<>>`},
		{"param \"${ok}\": not present in body", `body: "foofball" param: <key: "${ok}" value:<>>`},
		{"param \"${foof}\": schema: is nil", `body: "${foof}ball" param: <key: "${foof}" value:<>>`},
		{"param \"${foof}\": schema: has no type", `body: "${foof}ball" param: <key: "${foof}" value: <schema: <>>>`},

		{`param "${foof}": default value: type is "str", expected "int"`, `body: "${foof}ball" param: <key: "${foof}" value: <schema: <int:<>> default: <str: "">>>`},
		{"param \"${foof}\": default value: not nullable", `body: "${foof}ball" param: <key: "${foof}" value: <schema: <int:<>> default: <null: <>>>>`},
		{"param \"${foof}\": default value: invalid character 'q'", `body: "${foof}ball" param: <key: "${foof}" value: <schema: <object:<>> default: <object: "querp">>>`},

		{`param "${f}": schema: set requires entries`, `body: "${f}" param: <key: "${f}" value: <schema: <enum:<>>>>`},
		{`param "${f}": schema: blank token`, `body: "${f}" param: <key: "${f}" value: <schema: <enum:<entry: <>>>>>`},
		{`param "${f}": schema: duplicate token "tok"`, `body: "{\"k\": ${f}}" param: <key: "${f}" value: <schema: <enum:<entry: <token: "tok"> entry: <token: "tok">>>>>`},

		{"parsing rendered body: invalid character 'w'", `body: "wumpus"`},
	}

	ftt.Run("File_Template.Normalize", t, func(t *ftt.Test) {
		t.Run("bad", func(t *ftt.Test) {
			for _, tc := range badCases {
				t.Run(tc.err, func(t *ftt.Test) {
					template := parse(t, tc.data)
					assert.Loosely(t, template.Normalize(), should.ErrLike(tc.err))
				})
			}

			t.Run("length", func(t *ftt.Test) {
				t.Run("string", func(t *ftt.Test) {
					template := parse(t, `
					body: "{\"key\": ${foof}}"
					param: <
						key: "${foof}"
						value: <
							schema: <str:<max_length: 1>>
						>
					>
					`)
					assert.Loosely(t, template.Normalize(), should.BeNil)
					_, err := template.RenderL(LiteralMap{"${foof}": "hi there"})
					assert.Loosely(t, err, should.ErrLike("param \"${foof}\": value is too large"))
				})

				t.Run("bytes", func(t *ftt.Test) {
					template := parse(t, `
					body: "{\"key\": ${foof}}"
					param: <
						key: "${foof}"
						value: <
							schema: <bytes:<max_length: 1>>
						>
					>
					`)
					assert.Loosely(t, template.Normalize(), should.BeNil)
					_, err := template.RenderL(LiteralMap{"${foof}": []byte("hi there")})
					assert.Loosely(t, err, should.ErrLike("param \"${foof}\": value is too large"))
				})

				t.Run("object", func(t *ftt.Test) {
					template := parse(t, `
					body: "{\"key\": ${foof}}"
					param: <
						key: "${foof}"
						value: <
							schema: <object:<max_length: 4>>
						>
					>
					`)
					assert.Loosely(t, template.Normalize(), should.BeNil)
					_, err := template.RenderL(LiteralMap{"${foof}": map[string]any{"hi": 1}})
					assert.Loosely(t, err, should.ErrLike("param \"${foof}\": value is too large"))
				})

				t.Run("enum", func(t *ftt.Test) {
					template := parse(t, `
					body: "{\"key\": ${foof}}"
					param: <
						key: "${foof}"
						value: <
							schema: <enum:<
								entry: <token: "foo">
							>>
						>
					>
					`)
					assert.Loosely(t, template.Normalize(), should.BeNil)
					_, err := template.RenderL(LiteralMap{"${foof}": "bar"})
					assert.Loosely(t, err, should.ErrLike("param \"${foof}\": value does not match enum: \"bar\""))
				})
			})

			t.Run("parse", func(t *ftt.Test) {
				t.Run("not obj value", func(t *ftt.Test) {
					m, err := (LiteralMap{"$key": &Value_Object{"querp"}}).Convert()
					assert.Loosely(t, err, should.BeNil)
					spec := &Specifier{TemplateName: "thing", Params: m}
					assert.Loosely(t, spec.Normalize(), should.ErrLike("param \"$key\": invalid character 'q'"))
				})

				t.Run("not ary value", func(t *ftt.Test) {
					m, err := (LiteralMap{"$key": &Value_Array{"querp"}}).Convert()
					assert.Loosely(t, err, should.BeNil)
					spec := &Specifier{TemplateName: "thing", Params: m}
					assert.Loosely(t, spec.Normalize(), should.ErrLike("param \"$key\": invalid character 'q'"))
				})
			})

			t.Run("required param", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <int: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				_, err := template.Render(nil)
				assert.Loosely(t, err, should.ErrLike("param \"${value}\": missing"))
			})

			t.Run("extra param", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <int: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				_, err := template.RenderL(LiteralMap{"foo": nil, "${value}": 1})
				assert.Loosely(t, err, should.ErrLike("unknown parameters: [\"foo\"]"))
			})

			t.Run("bad type", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <int: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				_, err := template.RenderL(LiteralMap{"${value}": nil})
				assert.Loosely(t, err, should.ErrLike("param \"${value}\": not nullable"))
			})

			t.Run("prevents JSONi", func(t *ftt.Test) {
				t.Run("object", func(t *ftt.Test) {
					m, err := (LiteralMap{
						"${value}": &Value_Object{`{}, "otherKey": {}`}}).Convert()
					assert.Loosely(t, err, should.BeNil)
					spec := &Specifier{TemplateName: "thing", Params: m}
					assert.Loosely(t, spec.Normalize(), should.ErrLike("param \"${value}\": got extra junk"))

					spec.Params["${value}"].Value.(*Value_Object).Object = `{"extra": "space"}      `
					assert.Loosely(t, spec.Normalize(), should.BeNil)
					assert.Loosely(t, spec.Params["${value}"].GetObject(), should.Equal(`{"extra":"space"}`))
				})

				t.Run("array", func(t *ftt.Test) {
					va := &Value_Array{`[], "otherKey": []`}
					assert.Loosely(t, va.Normalize(), should.ErrLike("got extra junk"))
				})
			})
		})

		t.Run("good", func(t *ftt.Test) {
			t.Run("default", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <int: <>>
						default: <int: 20>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.Render(nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": 20}`))
			})

			t.Run("big int", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <int: <>>
						default: <int: 9223372036854775807>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.Render(nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": "9223372036854775807"}`))
			})

			t.Run("big uint", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <uint: <>>
						default: <uint: 18446744073709551615>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.Render(nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": "18446744073709551615"}`))
			})

			t.Run("param", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <uint: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.RenderL(LiteralMap{"${value}": uint(19)})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": 19}`))
			})

			t.Run("float", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <float: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.RenderL(LiteralMap{"${value}": 19.1})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": 19.1}`))
			})

			t.Run("boolean", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <bool: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.RenderL(LiteralMap{"${value}": true})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": true}`))
			})

			t.Run("array", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <array: <>>
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.RenderL(LiteralMap{"${value}": []any{"hi", 20}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": ["hi",20]}`))
			})

			t.Run("null", func(t *ftt.Test) {
				template := parse(t, `
				body: "{\"key\": ${value}}"
				param: <
					key: "${value}"
					value: <
						schema: <int: <>>
						nullable: true
					>
				>
				`)
				assert.Loosely(t, template.Normalize(), should.BeNil)
				doc, err := template.RenderL(LiteralMap{"${value}": nil})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, doc, should.Equal(`{"key": null}`))
			})
		})
	})
}
