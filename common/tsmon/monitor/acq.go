// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package monitor

const (
	metricNamePrefix = "/chrome/infra/"
)

func runningZeroes(values []int64) []int64 {
	ret := []int64{}

	var count int64
	for _, v := range values {
		if v == 0 {
			count++
		} else {
			if count != 0 {
				ret = append(ret, -count)
				count = 0
			}
			ret = append(ret, v)
		}
	}
	return ret
}
