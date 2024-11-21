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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParsePath(t *testing.T) {
	t.Parallel()

	ftt.Run("Expect path parsing", t, func(t *ftt.Test) {
		t.Run("succeeds when parsing scalar field", func(t *ftt.Test) {
			tryParsePath("str").andExpectPath(t, "str")
			tryParsePath("num").andExpectPath(t, "num")
		})
		t.Run("fails when given subfield for a scalar field", func(t *ftt.Test) {
			tryParsePath("str.str").andExpectErrorLike(t, "scalar field cannot have subfield: \"str\"")
		})
		t.Run("fails for invalid delimiter", func(t *ftt.Test) {
			tryParsePath("str@").andExpectErrorLike(t, "expected delimiter: .; got @")
		})
		t.Run("succeeds when parsing repeated field", func(t *ftt.Test) {
			tryParsePath("strs").andExpectPath(t, "strs")
			tryParsePath("nums").andExpectPath(t, "nums")
			tryParsePath("msgs").andExpectPath(t, "msgs")
		})
		t.Run("succeeds when parsing repeated field and path ends with star", func(t *ftt.Test) {
			// trailing stars are kept
			tryParsePath("strs.*").andExpectPath(t, "strs", "*")
			tryParsePath("nums.*").andExpectPath(t, "nums", "*")
			tryParsePath("msgs.*").andExpectPath(t, "msgs", "*")
		})
		t.Run("fails when parsing repeated field and path ends with index", func(t *ftt.Test) {
			tryParsePath("strs.1").andExpectErrorLike(t, "expected a star following a repeated field; got token: \"1\"")
			tryParsePath("nums.2").andExpectErrorLike(t, "expected a star following a repeated field; got token: \"2\"")
			tryParsePath("msgs.a").andExpectErrorLike(t, "expected a star following a repeated field; got token: \"a\"")
		})
		t.Run("succeeds when parsing map field (key type integer, scalar value)", func(t *ftt.Test) {
			tryParsePath("map_num_str.1").andExpectPath(t, "map_num_str", "1")
			tryParsePath("map_num_str.-1").andExpectPath(t, "map_num_str", "-1")
			tryParsePath("map_num_str.*").andExpectPath(t, "map_num_str", "*")
		})
		t.Run("succeeds when parsing map field (key type string, scalar value)", func(t *ftt.Test) {
			// unquoted
			tryParsePath("map_str_num.abcd").andExpectPath(t, "map_str_num", "abcd")
			tryParsePath("map_str_num._ab_cd").andExpectPath(t, "map_str_num", "_ab_cd")
			// quoted
			tryParsePath("map_str_num.`abcd`").andExpectPath(t, "map_str_num", "abcd")
			tryParsePath("map_str_num.`_ab.cd`").andExpectPath(t, "map_str_num", "_ab.cd")
			tryParsePath("map_str_num.`ab``cd`").andExpectPath(t, "map_str_num", "ab`cd")
			tryParsePath("map_str_num.*").andExpectPath(t, "map_str_num", "*")
		})
		t.Run("succeeds when parsing map field (key type boolean, scalar value)", func(t *ftt.Test) {
			tryParsePath("map_bool_str.false").andExpectPath(t, "map_bool_str", "false")
			tryParsePath("map_bool_str.true").andExpectPath(t, "map_bool_str", "true")
			tryParsePath("map_bool_str.*").andExpectPath(t, "map_bool_str", "*")
		})
		t.Run("fails when parsing map field with incompatible key type", func(t *ftt.Test) {
			tryParsePath("map_num_str.a").andExpectErrorLike(t, "expected map key kind int32; got token: \"a\"")
			tryParsePath("map_str_num.1").andExpectErrorLike(t, "expected map key kind string; got token: \"1\"")
			tryParsePath("map_bool_str.not_a_bool").andExpectErrorLike(t, "expected map key kind bool; got token: \"not_a_bool\"")
		})
		t.Run("succeeds when parsing map field (value type message)", func(t *ftt.Test) {
			tryParsePath("map_str_msg.some_key.str").andExpectPath(t, "map_str_msg",
				"some_key", "str")
			tryParsePath("map_str_msg.*.str").andExpectPath(t, "map_str_msg", "*", "str")
		})
		t.Run("succeeds when parsing message field", func(t *ftt.Test) {
			tryParsePath("msg.str").andExpectPath(t, "msg", "str")
			tryParsePath("msg.*").andExpectPath(t, "msg", "*")
			tryParsePath("msg.msg.msg.*").andExpectPath(t, "msg", "msg", "msg", "*")
		})
		t.Run("fails when parsing message field and given a non-string field name", func(t *ftt.Test) {
			tryParsePath("msg.123").andExpectErrorLike(t, "expected a field name of type string; got token: \"123\"")
		})
		t.Run("fails when parsing message field and star is not the last token", func(t *ftt.Test) {
			tryParsePath("msg.*.str").andExpectErrorLike(t, "expected end of string; got token: \"str\"")
		})
		t.Run("fails when parsing message field with unknown subfield", func(t *ftt.Test) {
			tryParsePath("msg.unknown_field").andExpectErrorLike(t, fmt.Sprintf("field \"unknown_field\" does not exist in message %s", testMsgDescriptor.Name()))
			tryParsePath("msg.msg.unknown_field").andExpectErrorLike(t, fmt.Sprintf("field \"unknown_field\" does not exist in message %s", testMsgDescriptor.Name()))
		})
		t.Run("succeeds when parsing repeated message fields given subfield", func(t *ftt.Test) {
			tryParsePath("msgs.*.str").andExpectPath(t, "msgs", "*", "str")
		})
		t.Run("fails when ends delimiter", func(t *ftt.Test) {
			tryParsePath("msg.").andExpectErrorLike(t, "path can't end with delimiter: .")
		})
		t.Run("fails when quoted string is not closed", func(t *ftt.Test) {
			tryParsePath("`quoted``str").andExpectErrorLike(t, "a quoted string is never closed; got: \"quoted`str\"")
		})
		t.Run("fails when an integer literal has only minus sign", func(t *ftt.Test) {
			tryParsePath("map_num_str.-").andExpectErrorLike(t, "expected digit following minus sign for negative numbers; got minus sign only")
		})
		t.Run("fails when multiple delimiters", func(t *ftt.Test) {
			tryParsePath("msg..str").andExpectErrorLike(t, "unexpected token: .")
			tryParsePath("msg..").andExpectErrorLike(t, "unexpected token: .")
		})
	})
}

// parseResult is a helper struct to enable descriptive test for path parsing
type parseResult struct {
	p   path
	err error
}

func tryParsePath(rawPath string) parseResult {
	p, err := parsePath(rawPath, testMsgDescriptor, true)
	return parseResult{
		p:   p,
		err: err,
	}
}

func (res parseResult) andExpectErrorLike(t testing.TB, errorSubstring string) {
	t.Helper()
	assert.Loosely(t, res.err, should.ErrLike(errorSubstring), truth.LineContext())
}

func (res parseResult) andExpectPath(t testing.TB, segments ...string) {
	t.Helper()
	assert.Loosely(t, res.err, should.BeNil, truth.LineContext())
	expectedPath := make(path, len(segments))
	for i, seg := range segments {
		expectedPath[i] = seg
	}
	assert.Loosely(t, res.p, should.Resemble(expectedPath), truth.LineContext())
}
