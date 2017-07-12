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

package nestedflagset

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLexer(t *testing.T) {
	Convey("Given a lexer with an empty value", t, func() {
		l := lexer("", ':')

		Convey("The lexer should start in a finished state.", func() {
			So(l.finished(), ShouldBeTrue)
		})

		Convey("The next token should be empty.", func() {

			So(l.nextToken(), ShouldEqual, token(""))
		})

		Convey("Splitting should yield a zero-length token slice.", func() {
			So(l.split(), ShouldResemble, []token{})
		})
	})

	s := `a: b c:d":e:ł:f":g:ü:h\"i:j\":k`
	Convey(`Given a complex test string: `+s, t, func() {
		l := lexer(s, ':')

		Convey("Should yield the expected token set and be finished.", func() {
			expectedTokens := []token{
				`a`,
				` b c`,
				`d:e:ł:f`,
				`g`,
				`ü`,
				`h"i`,
				`j"`,
				`k`,
			}
			for _, expected := range expectedTokens {
				So(l.nextToken(), ShouldEqual, expected)
			}

			So(l.finished(), ShouldBeTrue)
			So(l.nextToken(), ShouldEqual, token(""))
		})
	})

	Convey("Given a simple lexer string: a:b:c", t, func() {
		l := lexer(`a:b:c`, ':')

		Convey("Split should yield three elements.", func() {
			So(l.split(), ShouldResemble, []token{"a", "b", "c"})
		})
	})
}
