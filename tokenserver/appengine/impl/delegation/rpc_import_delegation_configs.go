// Copyright 2016 The LUCI Authors.
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
