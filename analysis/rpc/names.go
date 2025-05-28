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

package rpc

import (
	"fmt"
	"net/url"
	"regexp"

	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/pbutil"
)

// Regular expressions for matching resource names used in APIs.
var (
	GenericKeyPattern = "[a-z0-9\\-]+"
	// ClusterNameRe performs partial validation of a cluster resource name.
	// Cluster algorithm and ID must be further validated by
	// ClusterID.Validate().
	ClusterNameRe = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/clusters/(` + GenericKeyPattern + `)/(` + GenericKeyPattern + `)$`)
	// ClusterFailuresNameRe performs a partial validation of the resource
	// name for a cluster's failures.
	// Cluster algorithm and ID must be further validated by
	// ClusterID.Validate().
	ClusterFailuresNameRe = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/clusters/(` + GenericKeyPattern + `)/(` + GenericKeyPattern + `)/failures$`)
	// ClusterExoneratedTestVariantsNameRe and
	// ClusterExoneratedTestVariantBranchesNameRe performs a partial
	// validation of the resource name for a cluster's exonerated
	// test variant (branches).
	// Cluster algorithm and ID must be further validated by
	// ClusterID.Validate().
	ClusterExoneratedTestVariantsNameRe        = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/clusters/(` + GenericKeyPattern + `)/(` + GenericKeyPattern + `)/exoneratedTestVariants$`)
	ClusterExoneratedTestVariantBranchesNameRe = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/clusters/(` + GenericKeyPattern + `)/(` + GenericKeyPattern + `)/exoneratedTestVariantBranches$`)
	ProjectNameRe                              = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)$`)
	ProjectConfigNameRe                        = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/config$`)
	ProjectMetricNameRe                        = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/metrics/(` + metrics.MetricIDPattern + `)$`)
	ReclusteringProgressNameRe                 = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/reclusteringProgress$`)
	RuleNameRe                                 = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/rules/(` + rules.RuleIDRePattern + `)$`)
	// TestVariantBranchNameRe performs a partial validation of a TestVariantBranch name.
	// Test ID must be further validated with ValidateTestID.
	TestVariantBranchNameRe = regexp.MustCompile(`^projects/(` + pbutil.ProjectRePattern + `)/tests/([^/]+)/variants/(` + config.VariantHashRePattern + `)/refs/(` + config.RefHashRePattern + `)$`)
)

// parseProjectName parses a project resource name into a project ID.
func parseProjectName(name string) (project string, err error) {
	match := ProjectNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", errors.New("invalid project name, expected format: projects/{project}")
	}
	return match[1], nil
}

// parseProjectConfigName parses a project config resource name into a project ID.
func parseProjectConfigName(name string) (project string, err error) {
	match := ProjectConfigNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", errors.New("invalid project config name, expected format: projects/{project}/config")
	}
	return match[1], nil
}

// parseProjectMetricName parses a project metric name into its constituent
// project and metric ID parts.
func parseProjectMetricName(name string) (project string, metricID metrics.ID, err error) {
	match := ProjectMetricNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", "", errors.New("invalid project metric name, expected format: projects/{project}/metrics/{metric_id}")
	}
	return match[1], metrics.ID(match[2]), nil
}

// parseReclusteringProgressName parses a reclustering progress resource name
// into its constituent project ID part.
func parseReclusteringProgressName(name string) (project string, err error) {
	match := ReclusteringProgressNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", errors.New("invalid reclustering progress name, expected format: projects/{project}/reclusteringProgress")
	}
	return match[1], nil
}

// parseRuleName parses a rule resource name into its constituent ID parts.
func parseRuleName(name string) (project, ruleID string, err error) {
	match := RuleNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", "", errors.New("invalid rule name, expected format: projects/{project}/rules/{rule_id}")
	}
	return match[1], match[2], nil
}

// parseClusterName parses a cluster resource name into its constituent ID
// parts. Algorithm aliases are resolved to concrete algorithm names.
func parseClusterName(name string) (project string, clusterID clustering.ClusterID, err error) {
	if name == "" {
		return "", clustering.ClusterID{}, errors.New("must be specified")
	}
	match := ClusterNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", clustering.ClusterID{}, errors.New("invalid cluster name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}")
	}
	algorithm := resolveAlgorithm(match[2])
	id := match[3]
	cID := clustering.ClusterID{Algorithm: algorithm, ID: id}
	if err := cID.Validate(); err != nil {
		return "", clustering.ClusterID{}, errors.Fmt("invalid cluster identity: %w", err)
	}
	return match[1], cID, nil
}

// parseClusterFailuresName parses the resource name for a cluster's failures
// into its constituent ID parts. Algorithm aliases are resolved to
// concrete algorithm names.
func parseClusterFailuresName(name string) (project string, clusterID clustering.ClusterID, err error) {
	match := ClusterFailuresNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", clustering.ClusterID{}, errors.New("invalid cluster failures name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}/failures")
	}
	algorithm := resolveAlgorithm(match[2])
	id := match[3]
	cID := clustering.ClusterID{Algorithm: algorithm, ID: id}
	if err := cID.Validate(); err != nil {
		return "", clustering.ClusterID{}, errors.Fmt("cluster id: %w", err)
	}
	return match[1], cID, nil
}

// parseClusterExoneratedTestVariantsName parses the resource name for a cluster's
// exonerated test variants into its constituent ID parts. Algorithm aliases are
// resolved to concrete algorithm names.
func parseClusterExoneratedTestVariantsName(name string) (project string, clusterID clustering.ClusterID, err error) {
	match := ClusterExoneratedTestVariantsNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", clustering.ClusterID{}, errors.New("invalid resource name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}/exoneratedTestVariants")
	}
	algorithm := resolveAlgorithm(match[2])
	id := match[3]
	cID := clustering.ClusterID{Algorithm: algorithm, ID: id}
	if err := cID.Validate(); err != nil {
		return "", clustering.ClusterID{}, errors.Fmt("cluster id: %w", err)
	}
	return match[1], cID, nil
}

// parseClusterExoneratedTestVariantBranchesName parses the resource name for a
// cluster's exonerated test variant branches into its constituent ID parts.
// Algorithm aliases are resolved to concrete algorithm names.
func parseClusterExoneratedTestVariantBranchesName(name string) (project string, clusterID clustering.ClusterID, err error) {
	match := ClusterExoneratedTestVariantBranchesNameRe.FindStringSubmatch(name)
	if match == nil {
		return "", clustering.ClusterID{}, errors.New("invalid resource name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}/exoneratedTestVariantBranches")
	}
	algorithm := resolveAlgorithm(match[2])
	id := match[3]
	cID := clustering.ClusterID{Algorithm: algorithm, ID: id}
	if err := cID.Validate(); err != nil {
		return "", clustering.ClusterID{}, errors.Fmt("cluster id: %w", err)
	}
	return match[1], cID, nil
}

// parseTestVariantBranchName parses the resource name into project, test_id,
// variant hash and ref hash.
func parseTestVariantBranchName(name string) (project, testID, variantHash, refHash string, err error) {
	matches := TestVariantBranchNameRe.FindStringSubmatch(name)
	if matches == nil || len(matches) != 5 {
		return "", "", "", "", errors.New("name must be of format projects/{PROJECT}/tests/{URL_ESCAPED_TEST_ID}/variants/{VARIANT_HASH}/refs/{REF_HASH}")
	}
	// Unescape test_id.
	testID, err = url.PathUnescape(matches[2])
	if err != nil {
		return "", "", "", "", errors.Fmt("malformed test id: %w", err)
	}

	if err := rdbpbutil.ValidateTestID(testID); err != nil {
		return "", "", "", "", errors.Fmt("test id %q: %w", testID, err)
	}

	return matches[1], testID, matches[3], matches[4], nil
}

// ruleName constructs a rule resource name from its components.
func ruleName(project, ruleID string) string {
	return fmt.Sprintf("projects/%s/rules/%s", project, ruleID)
}

// testVariantBranchName constructs a test variant branch resource name
// from its components.
func testVariantBranchName(project, testID, variantHash, refHash string) string {
	encodedTestID := url.PathEscape(testID)
	return fmt.Sprintf("projects/%s/tests/%s/variants/%s/refs/%s", project, encodedTestID, variantHash, refHash)
}
