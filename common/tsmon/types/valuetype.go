// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

// ValueType is an enum for the type of a metric.
type ValueType int

// Types of metric values.
const (
	NonCumulativeIntType ValueType = iota
	CumulativeIntType
	NonCumulativeFloatType
	CumulativeFloatType
	StringType
	BoolType
	NonCumulativeDistributionType
	CumulativeDistributionType
)
