// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// ImportDelegationConfigsRPC implements Admin.ImportDelegationConfigs method.
type ImportDelegationConfigsRPC struct {
	RulesCache *RulesCache // usually GlobalRulesCache, but replaced in tests
}

// ImportDelegationConfigs fetches configs from from luci-config right now.
func (r *ImportDelegationConfigsRPC) ImportDelegationConfigs(c context.Context, _ *empty.Empty) (*admin.ImportedConfigs, error) {
	rev, err := r.RulesCache.ImportConfigs(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to fetch delegation configs")
		return nil, grpc.Errorf(codes.Internal, err.Error())
	}
	return &admin.ImportedConfigs{Revision: rev}, nil
}
