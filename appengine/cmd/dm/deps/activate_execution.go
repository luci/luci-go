// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
)

func (d *deps) ActivateExecution(c context.Context, req *dm.ActivateExecutionReq) (*google_pb.Empty, error) {
	err := datastore.Get(c).RunInTransaction(func(c context.Context) error {
		_, _, err := model.ActivateExecution(c, req.Auth, req.ExecutionToken)
		return err
	}, nil)
	return &google_pb.Empty{}, err
}
