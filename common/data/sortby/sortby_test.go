// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package sortby

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("sortby", t, func() {
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

		So(s, ShouldResemble, CoolStructSlice{
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
		})
	})
}
