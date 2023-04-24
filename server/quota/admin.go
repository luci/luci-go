// Copyright 2022 The LUCI Authors.
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

package quota

import (
	"context"

	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/server/quota/quotapb"
)

// Ensure quotaAdmin implements QuotaAdminServer at compile-time.
var _ quotapb.AdminServer = &quotaAdmin{}

// quotaAdmin implements quotapb.QuotaAdminServer.
type quotaAdmin struct{}

// GetAccounts returns the indicated Accounts.
func (q *quotaAdmin) GetAccounts(ctx context.Context, req *quotapb.GetAccountsRequest) (rsp *quotapb.GetAccountsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	panic("implement me")
}

// ApplyOps applies the given Ops to the Quota state.
func (q *quotaAdmin) ApplyOps(ctx context.Context, req *quotapb.ApplyOpsRequest) (rsp *quotapb.ApplyOpsResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	panic("implement me")
}

// WritePolicyConfig ingests the given config into the database.
func (q *quotaAdmin) WritePolicyConfig(ctx context.Context, req *quotapb.WritePolicyConfigRequest) (rsp *quotapb.WritePolicyConfigResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()
	panic("implement me")
}
