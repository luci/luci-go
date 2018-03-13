// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// ImportServiceAccountsConfigsRPC implements admin.ImportServiceAccountsConfigs
// method.
type ImportServiceAccountsConfigsRPC struct {
	RulesCache *RulesCache // usually GlobalRulesCache, but replaced in tests
}

// ImportServiceAccountsConfigs fetches configs from from luci-config right now.
func (r *ImportServiceAccountsConfigsRPC) ImportServiceAccountsConfigs(c context.Context, _ *empty.Empty) (*admin.ImportedConfigs, error) {
	rev, err := r.RulesCache.ImportConfigs(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to fetch service accounts configs")
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return &admin.ImportedConfigs{Revision: rev}, nil
}

// SetupConfigValidation registers the config validation rules.
func (r *ImportServiceAccountsConfigsRPC) SetupConfigValidation(rules *validation.RuleSet) {
	r.RulesCache.SetupConfigValidation(rules)
}
