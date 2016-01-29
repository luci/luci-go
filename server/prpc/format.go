// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	prpccommon "github.com/luci/luci-go/common/prpc"
)

// acceptFormat is a format specified in "Accept" header.
type acceptFormat struct {
	Format        prpccommon.Format
	QualityFactor float32 // preference, range: [0.0, 0.1]
}

// acceptFormatSlice is sortable by quality factor (desc) and format.
type acceptFormatSlice []acceptFormat

func (s acceptFormatSlice) Len() int {
	return len(s)
}

func (s acceptFormatSlice) Less(i, j int) bool {
	a, b := s[i], s[j]
	const epsilon = 0.000000001
	// quality factor descending
	if a.QualityFactor+epsilon > b.QualityFactor {
		return true
	}
	if a.QualityFactor+epsilon < b.QualityFactor {
		return false
	}
	// format ascending
	return a.Format < b.Format
}

func (s acceptFormatSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
