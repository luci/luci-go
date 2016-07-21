// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/logging"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
)

func (d *deps) FinishAttempt(c context.Context, req *dm.FinishAttemptReq) (_ *google_pb.Empty, err error) {
	logging.Fields{"execution": req.Auth.Id}.Infof(c, "finishing")
	return &google_pb.Empty{}, tumbleNow(c, &mutate.FinishAttempt{
		FinishAttemptReq: *req})
}
