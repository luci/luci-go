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
	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"

	"github.com/luci/luci-go/tokenserver/appengine/impl/certconfig"
	"github.com/luci/luci-go/tokenserver/appengine/impl/delegation"
	"github.com/luci/luci-go/tokenserver/appengine/impl/machinetoken"
	"github.com/luci/luci-go/tokenserver/appengine/impl/serviceaccounts"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// serverImpl implements admin.AdminServer RPC interface.
type serverImpl struct {
	certconfig.ImportCAConfigsRPC
	delegation.ImportDelegationConfigsRPC
	serviceaccounts.ImportServiceAccountsConfigsRPC
	machinetoken.InspectMachineTokenRPC
	delegation.InspectDelegationTokenRPC
	serviceaccounts.InspectOAuthTokenGrantRPC
}

// NewServer returns prod AdminServer implementation.
//
// It assumes authorization has happened already.
func NewServer() admin.AdminServer {
	signer := gaesigner.Signer{}
	return &serverImpl{
		ImportDelegationConfigsRPC: delegation.ImportDelegationConfigsRPC{
			RulesCache: delegation.GlobalRulesCache,
		},
		InspectMachineTokenRPC: machinetoken.InspectMachineTokenRPC{
			Signer: signer,
		},
		InspectDelegationTokenRPC: delegation.InspectDelegationTokenRPC{
			Signer: signer,
		},
	}
}
