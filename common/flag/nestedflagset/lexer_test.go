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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLexer(t *testing.T) {
	ftt.Run("Given a lexer with an empty value", t, func(t *ftt.Test) {
		l := lexer("", ':')

		t.Run("The lexer should start in a finished state.", func(t *ftt.Test) {
			assert.Loosely(t, l.finished(), should.BeTrue)
		})

		t.Run("The next token should be empty.", func(t *ftt.Test) {

			assert.Loosely(t, l.nextToken(), should.Equal(token("")))
		})

		t.Run("Splitting should yield a zero-length token slice.", func(t *ftt.Test) {
			assert.Loosely(t, l.split(), should.Match([]token{}))
		})
	})

	s := `a: b c:d":e:ł:f":g:ü:h\"i:j\":k`
	ftt.Run(`Given a complex test string: `+s, t, func(t *ftt.Test) {
		l := lexer(s, ':')

		t.Run("Should yield the expected token set and be finished.", func(t *ftt.Test) {
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
				assert.Loosely(t, l.nextToken(), should.Equal(expected))
			}

			assert.Loosely(t, l.finished(), should.BeTrue)
			assert.Loosely(t, l.nextToken(), should.Equal(token("")))
		})
	})

	ftt.Run("Given a simple lexer string: a:b:c", t, func(t *ftt.Test) {
		l := lexer(`a:b:c`, ':')

		t.Run("Split should yield three elements.", func(t *ftt.Test) {
			assert.Loosely(t, l.split(), should.Match([]token{"a", "b", "c"}))
		})
	})
}
