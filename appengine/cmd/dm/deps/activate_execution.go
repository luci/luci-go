// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/grpcutil"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
)

func (d *deps) ActivateExecution(c context.Context, req *dm.ActivateExecutionReq) (*google_pb.Empty, error) {
	return &google_pb.Empty{}, grpcutil.Unimplemented
}
