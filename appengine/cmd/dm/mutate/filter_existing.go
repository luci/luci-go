// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// filterExisting removes the FwdDep objects which already exist.
//
// returns gRPC code error.
func filterExisting(c context.Context, fwdDeps []*model.FwdDep) ([]*model.FwdDep, error) {
	ret := make([]*model.FwdDep, 0, len(fwdDeps))

	err := datastore.Get(c).GetMulti(fwdDeps)
	if err == nil {
		return nil, nil
	}

	merr, ok := err.(errors.MultiError)
	if !ok {
		// dunno what this is
		logging.WithError(err).Errorf(c, "error filtering existing fwdDeps")
		return nil, grpcutil.Internal
	}

	for i, err := range merr {
		if err == nil {
			continue
		}
		ret = append(ret, fwdDeps[i])
	}

	return ret, nil
}
