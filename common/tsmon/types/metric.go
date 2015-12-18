// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"github.com/luci/luci-go/common/tsmon/field"
)

// Metric is the low-level interface provided by all metrics.
// Concrete types are defined in the "metrics" package.
type Metric interface {
	Name() string
	Fields() []field.Field
	ValueType() ValueType
}
