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
	PermGetInvocation      = realms.RegisterPermission("resultdb.invocations.get")
	PermGetTestExoneration = realms.RegisterPermission("resultdb.testExonerations.get")
	PermGetTestResult      = realms.RegisterPermission("resultdb.testResults.get")
	PermGetArtifact        = realms.RegisterPermission("resultdb.artifacts.get")
	PermGetBaseline        = realms.RegisterPermission("resultdb.baselines.get")
	// TODO (nqmtuan): Add resultdb.instructions.get to resultdb.reader role.
	// https://source.corp.google.com/h/chromium/infra/infra_superproject/+/main:data/config/configs/chrome-infra-auth/permissions.cfg;l=192;bpv=0
	PermGetInstruction = realms.RegisterPermission("resultdb.instructions.get")

	PermListTestExonerations        = realms.RegisterPermission("resultdb.testExonerations.list")
	PermListLimitedTestExonerations = realms.RegisterPermission("resultdb.testExonerations.listLimited")
	PermListTestResults             = realms.RegisterPermission("resultdb.testResults.list")
	PermListLimitedTestResults      = realms.RegisterPermission("resultdb.testResults.listLimited")
	PermListArtifacts               = realms.RegisterPermission("resultdb.artifacts.list")
	PermListTestMetadata            = realms.RegisterPermission("resultdb.testMetadata.list")
)

func init() {
	PermListTestMetadata.AddFlags(realms.UsedInQueryRealms)
}
