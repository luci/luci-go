// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// ImportServiceAccountsConfigsRPC implements Admin.ImportServiceAccountsConfigs
// method.
type ImportServiceAccountsConfigsRPC struct {
}

// ImportServiceAccountsConfigs fetches configs from from luci-config right now.
func (r *ImportServiceAccountsConfigsRPC) ImportServiceAccountsConfigs(c context.Context, _ *empty.Empty) (*admin.ImportedConfigs, error) {
	return nil, grpc.Errorf(codes.Unavailable, "not implemented")
}
