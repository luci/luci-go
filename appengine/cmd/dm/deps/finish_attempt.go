// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const resultMaxLength = 256 * 1024

func (d *deps) FinishAttempt(c context.Context, req *dm.FinishAttemptReq) (_ *google_pb.Empty, err error) {
	req.JsonResult, err = model.NormalizeJSONObject(resultMaxLength, req.JsonResult)
	if err != nil {
		logging.WithError(err).Infof(c, "failed to normalized json")
		return nil, grpcutil.Errf(codes.InvalidArgument, "resuld json had normalization error: %s", err.Error())
	}

	return nil, tumbleNow(c, &mutate.FinishAttempt{
		Auth:             req.Auth,
		Result:           req.JsonResult,
		ResultExpiration: req.Expiration.Time(),
	})
}
