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

// Package adminsrv implements Admin API.
//
// Code defined here is either invoked by an administrator or by the service
// itself (via cron jobs or task queues).
package adminsrv

import (
	"strings"

	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
	"go.chromium.org/luci/tokenserver/appengine/impl/delegation"
	"go.chromium.org/luci/tokenserver/appengine/impl/machinetoken"
	"go.chromium.org/luci/tokenserver/appengine/impl/projectscope"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccounts"
)

// AdminServer implements admin.AdminServer RPC interface.
type AdminServer struct {
	admin.UnsafeAdminServer

	certconfig.ImportCAConfigsRPC
	delegation.ImportDelegationConfigsRPC
	delegation.InspectDelegationTokenRPC
	machinetoken.InspectMachineTokenRPC
	projectscope.ImportProjectIdentityConfigsRPC
	serviceaccounts.ImportProjectOwnedAccountsConfigsRPC
}

// NewServer returns prod AdminServer implementation.
//
// It assumes authorization has happened already.
func NewServer(signer signing.Signer, cloudProject string) *AdminServer {
	return &AdminServer{
		ImportDelegationConfigsRPC: delegation.ImportDelegationConfigsRPC{
			RulesCache: delegation.GlobalRulesCache,
		},
		InspectDelegationTokenRPC: delegation.InspectDelegationTokenRPC{
			Signer: signer,
		},
		InspectMachineTokenRPC: machinetoken.InspectMachineTokenRPC{
			Signer: signer,
		},
		ImportProjectIdentityConfigsRPC: projectscope.ImportProjectIdentityConfigsRPC{
			UseStagingEmail: strings.HasSuffix(cloudProject, "-dev"),
		},
		ImportProjectOwnedAccountsConfigsRPC: serviceaccounts.ImportProjectOwnedAccountsConfigsRPC{
			MappingCache: serviceaccounts.GlobalMappingCache,
		},
	}
}
