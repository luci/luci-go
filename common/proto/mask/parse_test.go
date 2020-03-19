// Copyright 2020 The LUCI Authors.
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

package mask

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParsePath(t *testing.T) {
	Convey("Expect path parsing", t, func() {
		Convey("succeeds when parsing scalar field", func() {
			tryParsePath("str").andExpectPath("str")
			tryParsePath("num").andExpectPath("num")
		})
		Convey("fails when given subfield for a scalar field", func() {
			tryParsePath("str.str").andExpectErrorLike("scalar field cannot have subfield: \"str\"")
		})
		Convey("fails for invalid delimiter", func() {
			tryParsePath("str@").andExpectErrorLike("expected delimiter: .; got @")
		})
		Convey("succeeds when parsing repeated field", func() {
			tryParsePath("strs").andExpectPath("strs")
			tryParsePath("nums").andExpectPath("nums")
			tryParsePath("msgs").andExpectPath("msgs")
		})
		Convey("succeeds when parsing repeated field and path ends with star", func() {
			// trailing star will be removed
			tryParsePath("strs.*").andExpectPath("strs")
			tryParsePath("nums.*").andExpectPath("nums")
			tryParsePath("msgs.*").andExpectPath("msgs")
		})
		Convey("fails when parsing repeated field and path ends with index", func() {
			tryParsePath("strs.1").andExpectErrorLike("expected a star following a repeated field; got token: \"1\"")
			tryParsePath("nums.2").andExpectErrorLike("expected a star following a repeated field; got token: \"2\"")
			tryParsePath("msgs.a").andExpectErrorLike("expected a star following a repeated field; got token: \"a\"")
		})
		Convey("succeeds when parsing map field (key type integer, scalar value)", func() {
			tryParsePath("map_num_str.1").andExpectPath("map_num_str", "1")
			tryParsePath("map_num_str.-1").andExpectPath("map_num_str", "-1")
			// trailing star will be removed
			tryParsePath("map_num_str.*").andExpectPath("map_num_str")
		})
		Convey("succeeds when parsing map field (key type string, scalar value)", func() {
			// unquoted
			tryParsePath("map_str_num.abcd").andExpectPath("map_str_num", "abcd")
			tryParsePath("map_str_num._ab_cd").andExpectPath("map_str_num", "_ab_cd")
			// quoted
			tryParsePath("map_str_num.`abcd`").andExpectPath("map_str_num", "abcd")
			tryParsePath("map_str_num.`_ab.cd`").andExpectPath("map_str_num", "_ab.cd")
			tryParsePath("map_str_num.`ab``cd`").andExpectPath("map_str_num", "ab`cd")
			// trailing star will be removed
			tryParsePath("map_str_num.*").andExpectPath("map_str_num")
		})
		Convey("succeeds when parsing map field (key type boolean, scalar value)", func() {
			tryParsePath("map_bool_str.false").andExpectPath("map_bool_str", "false")
			tryParsePath("map_bool_str.true").andExpectPath("map_bool_str", "true")
			tryParsePath("map_bool_str.*").andExpectPath("map_bool_str")
		})
		Convey("fails when parsing map field with incompatible key type", func() {
			tryParsePath("map_num_str.a").andExpectErrorLike("expected map key kind int32; got token: \"a\"")
			tryParsePath("map_str_num.1").andExpectErrorLike("expected map key kind string; got token: \"1\"")
			tryParsePath("map_bool_str.not_a_bool").andExpectErrorLike("expected map key kind bool; got token: \"not_a_bool\"")
		})
		Convey("succeeds when parsing map field (value type message)", func() {
			tryParsePath("map_str_msg.some_key.str").andExpectPath("map_str_msg",
				"some_key", "str")
			tryParsePath("map_str_msg.*.str").andExpectPath("map_str_msg", "*", "str")
		})
		Convey("succeeds when parsing message field", func() {
			tryParsePath("msg.str").andExpectPath("msg", "str")
			tryParsePath("msg.*").andExpectPath("msg")
			tryParsePath("msg.msg.msg.*").andExpectPath("msg", "msg", "msg")
		})
		Convey("fails when parsing message field and given a non-string field name", func() {
			tryParsePath("msg.123").andExpectErrorLike("expected a field name of type string; got token: \"123\"")
		})
		Convey("fails when parsing message field and star is not the last token", func() {
			tryParsePath("msg.*.str").andExpectErrorLike("expected end of string; got token: \"str\"")
		})
		Convey("fails when parsing message field with unknown subfield", func() {
			tryParsePath("msg.unknown_field").andExpectErrorLike(fmt.Sprintf("field \"unknown_field\" does not exist in message %s", testMsgDescriptor.Name()))
			tryParsePath("msg.msg.unknown_field").andExpectErrorLike(fmt.Sprintf("field \"unknown_field\" does not exist in message %s", testMsgDescriptor.Name()))
		})
		Convey("succeeds when parsing repeated message fields given subfield", func() {
			tryParsePath("msgs.*.str").andExpectPath("msgs", "*", "str")
		})
		Convey("fails when ends delimiter", func() {
			tryParsePath("msg.").andExpectErrorLike("path can't end with delimiter: .")
		})
		Convey("fails when quoted string is not closed", func() {
			tryParsePath("`quoted``str").andExpectErrorLike("a quoted string is never closed; got: \"quoted`str\"")
		})
		Convey("fails when an integer literal has only minus sign", func() {
			tryParsePath("map_num_str.-").andExpectErrorLike("expected digit following minus sign for negative numbers; got minus sign only")
		})
		Convey("fails when multiple delimiters", func() {
			tryParsePath("msg..str").andExpectErrorLike("unexpected token: .")
			tryParsePath("msg..").andExpectErrorLike("unexpected token: .")
		})
		Convey("succeeds for json name", func() {
			p, err := parsePath("jsonName", testMsgDescriptor, true)
			So(err, ShouldBeNil)
			So(p, ShouldResemble, path{"json_name"})
			p, err = parsePath("another_json_name", testMsgDescriptor, true)
			So(err, ShouldBeNil)
			So(p, ShouldResemble, path{"json_name_option"})

		})
	})
}

// parseResult is a helper struct to enable descriptive test for path parsing
type parseResult struct {
	p   path
	err error
}

func tryParsePath(rawPath string) parseResult {
	p, err := parsePath(rawPath, testMsgDescriptor, false)
	return parseResult{
		p:   p,
		err: err,
	}
}

func (res parseResult) andExpectErrorLike(errorSubstring string) {
	So(res.err, ShouldErrLike, errorSubstring)
}

func (res parseResult) andExpectPath(segments ...string) {
	So(res.err, ShouldBeNil)
	expectedPath := make(path, len(segments))
	for i, seg := range segments {
		expectedPath[i] = seg
	}
	So(res.p, ShouldResemble, expectedPath)
}
