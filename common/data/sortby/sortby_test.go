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

	Convey("sortby", t, func() {
		s := CoolStructSlice{
			{194, 771, 222},
			{209, 28, 300},
			{413, 639, 772},
			{14, 761, 759},
			{866, 821, 447},
			{447, 373, 817},
			{132, 510, 149},
			{778, 513, 156},
			{713, 831, 596},
			{288, 83, 898},
			{679, 688, 903},
			{864, 100, 199},
			{229, 975, 648},
			{381, 290, 156},
			{447, 290, 156}, // intentionally partially equal
			{311, 614, 434},
		}

		sort.Sort(s)

		So(s, ShouldResemble, CoolStructSlice{
			{14, 761, 759},
			{132, 510, 149},
			{194, 771, 222},
			{209, 28, 300},
			{229, 975, 648},
			{288, 83, 898},
			{311, 614, 434},
			{381, 290, 156},
			{413, 639, 772},
			{447, 290, 156},
			{447, 373, 817},
			{679, 688, 903},
			{713, 831, 596},
			{778, 513, 156},
			{864, 100, 199},
			{866, 821, 447},
		})
	})
}
