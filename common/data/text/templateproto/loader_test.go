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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLoadFromConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadFile", t, func(t *ftt.Test) {
		templateContent := `
		template: <
			key: "hardcode"
			value: <
				doc: "it's hard-coded"
				body: <<EOF
					{"woot": ["sauce"]}
				EOF
			>
		>

		template: <
			key: "templ_1"
			value: <
				doc: << EOF
					This template is templ_1!
					It's pretty "exciting"!
				EOF
				body: << EOF
					{
						"json_key": ${json_key},
						"cmd": ["array", "of", ${thing}],
						"extra": ${extra}
					}
				EOF
				param: <
					key: "${json_key}"
					value: <
						doc: "it's a json key"
						schema: <int:<>>
					>
				>
				param: <
					key: "${thing}"
					value: <
						doc: << EOF
							${thing} represents a color or a fruit
						EOF
						schema: <enum:<
							entry: < doc: "fruit" token: "banana" >
							entry: < doc: "color" token: "white" >
							entry: < doc: "color" token: "purple" >
						>>
					>
				>
				param: <
					key: "${extra}"
					value: <
						nullable: true
						schema:<object:<>>
						default: <object: <<EOF
							{"yes": "please"}
						EOF
						>
					>
				>
			>
		>
		`

		t.Run("basic load", func(t *ftt.Test) {
			file, err := LoadFile(templateContent)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(&File{Template: map[string]*File_Template{
				"hardcode": {
					Doc:  "it's hard-coded",
					Body: `{"woot": ["sauce"]}`,
				},

				"templ_1": {
					Doc:  "This template is templ_1!\nIt's pretty \"exciting\"!",
					Body: "{\n\t\"json_key\": ${json_key},\n\t\"cmd\": [\"array\", \"of\", ${thing}],\n\t\"extra\": ${extra}\n}",
					Param: map[string]*File_Template_Parameter{
						"${json_key}": {
							Doc:    "it's a json key",
							Schema: &Schema{Schema: &Schema_Int{&Schema_Atom{}}},
						},
						"${thing}": {
							Doc: "${thing} represents a color or a fruit",
							Schema: &Schema{
								Schema: &Schema_Enum{&Schema_Set{Entry: []*Schema_Set_Entry{
									{Doc: "fruit", Token: "banana"},
									{Doc: "color", Token: "white"},
									{Doc: "color", Token: "purple"},
								}}},
							},
						},
						"${extra}": {
							Nullable: true,
							Schema:   &Schema{Schema: &Schema_Object{&Schema_JSON{}}},
							Default:  MustNewValue(map[string]any{"yes": "please"}),
						},
					},
				},
			}}))

			t.Run("basic render", func(t *ftt.Test) {
				ret, err := file.RenderL("templ_1", LiteralMap{"${thing}": "white", "${json_key}": 20})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Equal(`{
	"json_key": 20,
	"cmd": ["array", "of", "white"],
	"extra": {"yes":"please"}
}`))
			})

			t.Run("null override", func(t *ftt.Test) {
				ret, err := file.RenderL("templ_1", LiteralMap{"${thing}": "white", "${json_key}": 20, "${extra}": nil})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Equal(`{
	"json_key": 20,
	"cmd": ["array", "of", "white"],
	"extra": null
}`))
			})

			t.Run("bad render gets context", func(t *ftt.Test) {
				_, err := file.RenderL("templ_1", LiteralMap{"${thing}": 10, "${json_key}": 20, "${extra}": nil})
				assert.Loosely(t, err, should.ErrLike("rendering \"templ_1\": param \"${thing}\": type is \"int\", expected \"enum\""))
			})

			t.Run("hardcode", func(t *ftt.Test) {
				ret, err := file.RenderL("hardcode", nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, ret, should.Equal(`{"woot": ["sauce"]}`))
			})
		})
	})
}
