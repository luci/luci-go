// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package monitor

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRunningZeroes(t *testing.T) {
	data := []struct {
		values []int64
		want   []int64
	}{
		{[]int64{1, 0, 1}, []int64{1, -1, 1}},
		{[]int64{1, 0, 0, 1}, []int64{1, -2, 1}},
		{[]int64{1, 0, 0, 0, 1}, []int64{1, -3, 1}},
		{[]int64{1, 0, 1, 0, 2}, []int64{1, -1, 1, -1, 2}},
		{[]int64{1, 0, 1, 0, 0, 2}, []int64{1, -1, 1, -2, 2}},
		{[]int64{1, 0, 0, 1, 0, 0, 2}, []int64{1, -2, 1, -2, 2}},

		// Leading zeroes
		{[]int64{0, 1}, []int64{-1, 1}},
		{[]int64{0, 0, 1}, []int64{-2, 1}},
		{[]int64{0, 0, 0, 1}, []int64{-3, 1}},

		// Trailing zeroes
		{[]int64{1}, []int64{1}},
		{[]int64{1, 0}, []int64{1}},
		{[]int64{1, 0, 0}, []int64{1}},
		{[]int64{}, []int64{}},
		{[]int64{0}, []int64{}},
		{[]int64{0, 0}, []int64{}},
	}

	for i, d := range data {
		Convey(fmt.Sprintf("%d. runningZeroes(%v)", i, d.values), t, func() {
			got := runningZeroes(d.values)
			So(got, ShouldResemble, d.want)
		})
	}
}
