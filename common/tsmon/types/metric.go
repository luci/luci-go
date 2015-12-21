// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
)

// Metric is the low-level interface provided by all metrics.
// Concrete types are defined in the "metrics" package.
type Metric interface {
	Name() string
	Fields() []field.Field
	ValueType() ValueType
}

// DistributionMetric is the low-level interface provided by all distribution
// metrics.  It has a Bucketer which is responsible for assigning buckets to
// samples.  Concrete types are defined in the "metrics" package.
type DistributionMetric interface {
	Metric

	Bucketer() *distribution.Bucketer
}
