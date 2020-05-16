// Copyright 2020 The LUCI Authors.
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

package serviceaccountsv2

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// ImportProjectOwnedAccountsConfigsRPC implements the corresponding method.
type ImportProjectOwnedAccountsConfigsRPC struct {
	MappingCache *MappingCache // usually GlobalMappingCache, but replaced in tests
}

// ImportServiceAccountsConfigs fetches configs from luci-config right now.
func (r *ImportProjectOwnedAccountsConfigsRPC) ImportProjectOwnedAccountsConfigs(c context.Context, _ *empty.Empty) (*admin.ImportedConfigs, error) {
	// TODO
	return nil, grpc.Errorf(codes.Unimplemented, "not implemented yet")
}

// SetupConfigValidation registers the config validation rules.
func (r *ImportProjectOwnedAccountsConfigsRPC) SetupConfigValidation(rules *validation.RuleSet) {
	r.MappingCache.SetupConfigValidation(rules)
}
