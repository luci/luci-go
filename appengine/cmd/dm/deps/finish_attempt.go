// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"fmt"

	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/api/template"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const resultMaxLength = 256 * 1024

func (d *deps) FinishAttempt(c context.Context, req *dm.FinishAttemptReq) (_ *google_pb.Empty, err error) {
	if len(req.JsonResult) > resultMaxLength {
		return nil, fmt.Errorf("result payload is too large: %d > %d",
			len(req.JsonResult), resultMaxLength)
	}

	req.JsonResult, err = template.NormalizeJSON(req.JsonResult, true)
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
