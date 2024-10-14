// Copyright 2017 The LUCI Authors.
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

package sortby

import (
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type CoolStruct struct {
	A int
	B int
	C int

	Meta string
}

type CoolStructSlice []CoolStruct

func (s CoolStructSlice) Len() int      { return len(s) }
func (s CoolStructSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// these could be defined inline inside of the Less function, too.
func (s CoolStructSlice) LessA(i, j int) bool { return s[i].A < s[j].A }
func (s CoolStructSlice) LessB(i, j int) bool { return s[i].B < s[j].B }
func (s CoolStructSlice) LessC(i, j int) bool { return s[i].C < s[j].C }

func (s CoolStructSlice) Less(i, j int) bool {
	return Chain{s.LessA, s.LessB, nil, s.LessC}.Use(i, j)
}

func TestSortBy(t *testing.T) {
	t.Parallel()

	i := func(a, b, c int, meta ...string) CoolStruct {
		if len(meta) > 0 {
			return CoolStruct{a, b, c, meta[0]}
		}
		return CoolStruct{A: a, B: b, C: c}
	}

	ftt.Run("sortby", t, func(t *ftt.Test) {
		s := CoolStructSlice{
			i(194, 771, 222),
			i(209, 28, 300),
			i(413, 639, 772),
			i(14, 761, 759),
			i(866, 821, 447),
			i(447, 373, 817),
			i(132, 510, 149),
			i(778, 513, 156),
			i(713, 831, 596),
			i(288, 83, 898),
			i(679, 688, 903),
			i(864, 100, 199),
			i(132, 510, 149, "dup"), // include a duplicate
			i(229, 975, 648),
			i(381, 290, 156),
			i(447, 290, 156), // intentionally partially equal
			i(311, 614, 434),
		}

		sort.Stable(s)

		assert.Loosely(t, s, should.Resemble(CoolStructSlice{
			i(14, 761, 759),
			i(132, 510, 149),
			i(132, 510, 149, "dup"),
			i(194, 771, 222),
			i(209, 28, 300),
			i(229, 975, 648),
			i(288, 83, 898),
			i(311, 614, 434),
			i(381, 290, 156),
			i(413, 639, 772),
			i(447, 290, 156),
			i(447, 373, 817),
			i(679, 688, 903),
			i(713, 831, 596),
			i(778, 513, 156),
			i(864, 100, 199),
			i(866, 821, 447),
		}))
	})
}
