// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/grpcutil"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func (d *deps) EnsureAttempt(c context.Context, req *dm.EnsureAttemptReq) (_ *google_pb.Empty, err error) {
	ds := datastore.Get(c)

	if err = ds.Get(&model.Quest{ID: req.ToEnsure.Quest}); err != nil {
		return nil, grpcutil.Errf(codes.NotFound, "no such quest %q", req.ToEnsure.Quest)
	}

	return nil, tumbleNow(c, &mutate.EnsureAttempt{ID: req.ToEnsure})
}
