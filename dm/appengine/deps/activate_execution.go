// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/luci/luci-go/common/logging"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/mutate"
	"golang.org/x/net/context"
)

func (d *deps) ActivateExecution(c context.Context, req *dm.ActivateExecutionReq) (ret *empty.Empty, err error) {
	ret = &empty.Empty{}
	logging.Fields{"execution": req.Auth.Id}.Infof(c, "activating")
	err = tumbleNow(c, &mutate.ActivateExecution{
		Auth:   req.Auth,
		NewTok: req.ExecutionToken,
	})
	return
}
