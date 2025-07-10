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

package rdbperms

import (
	"go.chromium.org/luci/server/auth/realms"
)

var (
	PermGetArtifact        = realms.RegisterPermission("resultdb.artifacts.get")
	PermGetBaseline        = realms.RegisterPermission("resultdb.baselines.get")
	PermGetInstruction     = realms.RegisterPermission("resultdb.instructions.get")
	PermGetInvocation      = realms.RegisterPermission("resultdb.invocations.get")
	PermGetRootInvocation  = realms.RegisterPermission("resultdb.rootInvocations.get")
	PermGetTestExoneration = realms.RegisterPermission("resultdb.testExonerations.get")
	PermGetTestResult      = realms.RegisterPermission("resultdb.testResults.get")
	PermGetWorkUnit        = realms.RegisterPermission("resultdb.workUnits.get")

	PermListArtifacts               = realms.RegisterPermission("resultdb.artifacts.list")
	PermListTestExonerations        = realms.RegisterPermission("resultdb.testExonerations.list")
	PermListLimitedTestExonerations = realms.RegisterPermission("resultdb.testExonerations.listLimited")
	PermListTestResults             = realms.RegisterPermission("resultdb.testResults.list")
	PermListLimitedTestResults      = realms.RegisterPermission("resultdb.testResults.listLimited")
	PermListTestMetadata            = realms.RegisterPermission("resultdb.testMetadata.list")
	PermListWorkUnits               = realms.RegisterPermission("resultdb.workUnits.list")
	PermListLimitedWorkUnits        = realms.RegisterPermission("resultdb.workUnits.listLimited")
)

func init() {
	PermListTestMetadata.AddFlags(realms.UsedInQueryRealms)
	PermListArtifacts.AddFlags(realms.UsedInQueryRealms)
}
