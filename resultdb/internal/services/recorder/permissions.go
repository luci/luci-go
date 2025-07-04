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

package recorder

import (
	"go.chromium.org/luci/server/auth/realms"
)

var (
	permCreateRootInvocation = realms.RegisterPermission("resultdb.rootInvocations.create")

	permCreateWorkUnit = realms.RegisterPermission("resultdb.workUnits.create")

	permPutBaseline = realms.RegisterPermission("resultdb.baselines.put")

	permCreateInvocation       = realms.RegisterPermission("resultdb.invocations.create")
	permIncludeInvocation      = realms.RegisterPermission("resultdb.invocations.include")
	permSetSubmittedInvocation = realms.RegisterPermission("resultdb.invocations.setSubmitted")

	// Internal permissions
	permCreateWithReservedID               = realms.RegisterPermission("resultdb.invocations.createWithReservedID")
	permExportToBigQuery                   = realms.RegisterPermission("resultdb.invocations.exportToBigQuery")
	permSetProducerResource                = realms.RegisterPermission("resultdb.invocations.setProducerResource")
	permSetExportRoot                      = realms.RegisterPermission("resultdb.invocations.setExportRoot")
	permCreateRootInvocationWithReservedID = realms.RegisterPermission("resultdb.rootInvocations.createWithReservedID")
	permSetRootInvocationProducerResource  = realms.RegisterPermission("resultdb.rootInvocations.setProducerResource")
)
