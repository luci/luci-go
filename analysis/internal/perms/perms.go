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

// Package perms defines permissions used to control access to Weetbix
// resources, and related methods.
package perms

import (
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth/realms"
)

// All permissions in this file are checked against "<luciproject>:@root"
// realm, as rules and clusters do not live in any particular realm.

// Permissions that should usually be granted to all users that can view
// a project.
var (
	// Grants access to reading individual Weetbix rules in a LUCI project,
	// except for the rule definition (i.e. 'reason LIKE "%criteria%"'.).
	//
	// This also permits the user to see the identity of the configured
	// issue tracker for a project. (This is available via the URL
	// provided for bugs on a rule and via a separate config RPC.)
	PermGetRule = realms.RegisterPermission("weetbix.rules.get")

	// Grants access to listing all rules in a LUCI project,
	// except for the rule definition (i.e. 'reason LIKE "%criteria%"'.).
	//
	// This also permits the user to see the identity of the configured
	// issue tracker for a project. (This is available via the URL
	// provided for bugs on a rule.)
	PermListRules = realms.RegisterPermission("weetbix.rules.list")

	// Grants permission to get a cluster in a project.
	// This encompasses the cluster ID and aggregated impact for
	// the cluster (over all failures, not just those the user can see).
	//
	// Seeing failures in a cluster is contingent on also having
	// having "resultdb.testResults.list" permission in ResultDB
	// for the realm of the test result.
	//
	// This permission also allows the user to obtain Weetbix's
	// progress reclustering failures to reflect new rules, configuration
	// and algorithms.
	PermGetCluster = realms.RegisterPermission("weetbix.clusters.get")

	// Grants permission to list all clusters in a project.
	// This encompasses the cluster identifier and aggregated impact for
	// the clusters (over all failures, not just those the user can see).
	// More detailed cluster information, including cluster definition
	// and failures is contingent on being able to see failures in the
	// cluster.
	PermListClusters = realms.RegisterPermission("weetbix.clusters.list")

	// PermGetClustersByFailure allows the user to obtain the cluster
	// identit(ies) matching a given failure.
	PermGetClustersByFailure = realms.RegisterPermission("weetbix.clusters.getByFailure")

	// Grants permission to get project configuration, such
	// as the configured monorail issue tracker. Controls the
	// visibility of the project in the Weetbix main page.
	//
	// Can be assumed this is also granted wherever a project has
	// a weetbix.rules.* or weetbix.clusters.* CRUD permission;
	// many parts of Weetbix rely on Weetbix configuration and
	// there is no need to perform gratuitous access checks.
	PermGetConfig = realms.RegisterPermission("weetbix.config.get")
)

// The following permission grants view access to the rule definition,
// which could be sensitive if test names or failure reasons reveal
// sensitive product or hardware data.
var (
	// Grants access to reading the rule definition of Weetbix rules.
	PermGetRuleDefinition = realms.RegisterPermission("weetbix.rules.getDefinition")
)

// Mutating permissions.
var (
	// Grants permission to create a rule.
	// Should be granted only to trusted project contributors.
	PermCreateRule = realms.RegisterPermission("weetbix.rules.create")

	// Grants permission to update all rules in a project.
	// Permission to update a rule also implies permission to get the rule
	// and view the rule definition as the modified rule is returned in the
	// response to the UpdateRule RPC.
	// Should be granted only to trusted project contributors.
	PermUpdateRule = realms.RegisterPermission("weetbix.rules.update")
)

// Permissions used to control costs.
var (
	// Grants permission to perform expensive queries (that hit BigQuery).
	PermExpensiveClusterQueries = realms.RegisterPermission("weetbix.clusters.expensiveQueries")
)

// Permissions used to control access to test results.
var ListTestResultsAndExonerations = []realms.Permission{
	rdbperms.PermListTestResults,
	rdbperms.PermListTestExonerations,
}

func init() {
	rdbperms.PermListTestResults.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListTestExonerations.AddFlags(realms.UsedInQueryRealms)
}
