// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package projectscope

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// ImportProjectScopedServiceAccountsConfigsRPC implements admin.ImportProjectScopedServiceAccountsConfigs
// method.
type ImportProjectScopedServiceAccountsConfigsRPC struct {
	RulesCache *RulesCache // usually GlobalRulesCache, but replaced in tests
}

// ImportProjectScopedServiceAccountsConfigs fetches configs from from luci-config right now.
func (r *ImportProjectScopedServiceAccountsConfigsRPC) ImportProjectScopedServiceAccountsConfigs(c context.Context, _ *empty.Empty) (*admin.ImportedConfigs, error) {
	rev, err := r.RulesCache.ImportConfigs(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to fetch project scoped service accounts configs")
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	return &admin.ImportedConfigs{Revision: rev}, nil
}

// SetupConfigValidation registers the config validation rules.
func (r *ImportProjectScopedServiceAccountsConfigsRPC) SetupConfigValidation(rules *validation.RuleSet) {
	r.RulesCache.SetupConfigValidation(rules)
}
