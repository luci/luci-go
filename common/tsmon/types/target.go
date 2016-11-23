// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package types

import (
	pbv1 "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v1"
	pbv2 "github.com/luci/luci-go/common/tsmon/ts_mon_proto_v2"
)

// A Target knows how to put information about itself in a MetricsData message.
type Target interface {
	PopulateProtoV1(d *pbv1.MetricsData)
	PopulateProto(d *pbv2.MetricsCollection)
	Hash() uint64
	Clone() Target
}
