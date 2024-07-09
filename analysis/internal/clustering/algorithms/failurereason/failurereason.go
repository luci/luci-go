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

// Package failurereason contains the failure reason clustering algorithm
// for LUCI Analysis.
//
// This algorithm removes ips, temp file names, numbers and other such tokens
// to cluster similar reasons together.
package failurereason

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
)

// AlgorithmVersion is the version of the clustering algorithm. The algorithm
// version should be incremented whenever existing test results may be
// clustered differently (i.e. Cluster(f) returns a different value for some
// f that may have been already ingested).
const AlgorithmVersion = 6

// AlgorithmName is the identifier for the clustering algorithm.
// LUCI Analysis requires all clustering algorithms to have a unique
// identifier. Must match the pattern ^[a-z0-9-.]{1,32}$.
//
// The AlgorithmName must encode the algorithm version, so that each version
// of an algorithm has a different name.
var AlgorithmName = fmt.Sprintf("%sv%v", clustering.FailureReasonAlgorithmPrefix, AlgorithmVersion)

// BugTemplate is the template for the content of bugs created for failure
// reason clusters. A list of test IDs is included to improve searchability
// by test name.
var BugTemplate = template.Must(template.New("reasonTemplate").Parse(
	`This bug is for all test failures where the primary error message is similiar to the following (ignoring numbers and hexadecimal values):
{{.FailureReason}}

The following test(s) were observed to have matching failures at this time (at most five examples listed):
{{range .TestIDs}}- {{.}}
{{end}}`))

// To match any 1 or more digit numbers, or hex values (often appear in temp
// file names or prints of pointers), which will be replaced.
var clusterExp = regexp.MustCompile(`[/+0-9a-zA-Z]{10,}=+|[\-0-9a-fA-F \t]{16,}|[0-9a-fA-Fx]{8,}|[0-9]+`)

// likeEscapeRewriter escapes \, % and _ so that they are not interpreted by LIKE
// pattern matching.
var likeEscapeRewriter = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likeUnescapeRewriter unescapes the special sequences \\, \% and \_
// used in LIKE expressions, so that literal text matched appears unescaped.
// This is used to make cluster definitions read more naturally on the UI,
// even if it introduces some ambiguity.
var likeUnescapeRewriter = strings.NewReplacer(`\\`, `\`, `\%`, `%`, `\_`, `_`)

// Algorithm represents an instance of the reason-based clustering
// algorithm.
type Algorithm struct{}

// Name returns the identifier of the clustering algorithm.
func (a *Algorithm) Name() string {
	return AlgorithmName
}

// clusterLike returns the reason LIKE expression that defines
// the cluster the given test result belongs to.
//
// By default only numbers, hexadecimals and base64-encoding-like
// sequences are stripped out when clustering. But using configurable
// masking patterns, it is possible to strip out other parts too.
func clusterLike(config *compiledcfg.ProjectConfig, failure *clustering.Failure) string {
	// Escape \, % and _ so that they are not interpreted by LIKE
	// pattern matching.
	likePattern := likeEscapeRewriter.Replace(failure.Reason.PrimaryErrorMessage)

	// Replace hexadecimal sequences with wildcard matches. This is technically
	// broader than our original cluster definition, but is more readable, and
	// usually ends up matching the exact same set of failures.
	likePattern = clusterExp.ReplaceAllString(likePattern, "%")

	// Apply configured masks.
	for _, re := range config.ReasonMaskPatterns {
		likePattern = applyMask(re, likePattern)
	}

	return likePattern
}

// applyMask applies the given masking regexp to an error message.
//
// The regular expression re must have exactly one
// capturing sub-expression, and the part of this expression
// which matches the errorMessage is replaced with the LIKE
// wildcard operator "%".
//
// Masking is applied to all non-overlapping matches.
func applyMask(re *regexp.Regexp, errorMessage string) string {
	matches := re.FindAllStringSubmatchIndex(errorMessage, -1)
	if len(matches) == 0 {
		return errorMessage
	}
	var builder strings.Builder
	builder.Grow(len(errorMessage))

	// Replace the text in the first capturing subexpression with "%".
	var startIndex int
	for _, match := range matches {
		matchStart := match[2]
		matchEnd := match[3]
		builder.WriteString(errorMessage[startIndex:matchStart])
		builder.WriteString("%")
		startIndex = matchEnd
	}
	builder.WriteString(errorMessage[startIndex:])
	return builder.String()
}

// clusterKey returns the unhashed key for the cluster. Absent an extremely
// unlikely hash collision, this value is the same for all test results
// in the cluster.
func clusterKey(config *compiledcfg.ProjectConfig, failure *clustering.Failure) string {
	// Use like expression as the clustering key.
	return clusterLike(config, failure)
}

// Cluster clusters the given test failure and returns its cluster ID (if it
// can be clustered) or nil otherwise.
func (a *Algorithm) Cluster(config *compiledcfg.ProjectConfig, failure *clustering.Failure) []byte {
	if failure.Reason == nil || failure.Reason.PrimaryErrorMessage == "" {
		return nil
	}
	id := clusterKey(config, failure)
	// sha256 hash the resulting string.
	h := sha256.Sum256([]byte(id))
	// Take first 16 bytes as the ID. (Risk of collision is
	// so low as to not warrant full 32 bytes.)
	return h[0:16]
}

// ClusterDescription returns a description of the cluster, for use when
// filing bugs, with the help of the given example failure.
func (a *Algorithm) ClusterDescription(config *compiledcfg.ProjectConfig, summary *clustering.ClusterSummary) (*clustering.ClusterDescription, error) {
	if summary.Example.Reason == nil || summary.Example.Reason.PrimaryErrorMessage == "" {
		return nil, errors.New("cluster summary must contain example with failure reason")
	}
	type templateData struct {
		FailureReason string
		TestIDs       []string
	}
	var input templateData

	// Quote and escape.
	primaryError := strconv.QuoteToGraphic(summary.Example.Reason.PrimaryErrorMessage)
	// Unquote, so we are left with the escaped error message only.
	primaryError = primaryError[1 : len(primaryError)-1]

	input.FailureReason = primaryError
	for _, t := range summary.TopTests {
		input.TestIDs = append(input.TestIDs, clustering.EscapeToGraphical(t))
	}
	var b bytes.Buffer
	if err := BugTemplate.Execute(&b, input); err != nil {
		return nil, err
	}

	return &clustering.ClusterDescription{
		Title:       primaryError,
		Description: b.String(),
	}, nil
}

// ClusterTitle returns a definition of the cluster, typically in
// the form of an unhashed clustering key which is common
// across all test results in a cluster. For display on the cluster
// page or cluster listing.
func (a *Algorithm) ClusterTitle(config *compiledcfg.ProjectConfig, example *clustering.Failure) string {
	if example.Reason == nil || example.Reason.PrimaryErrorMessage == "" {
		return ""
	}
	// Should match exactly the algorithm in Cluster(...)
	key := clusterKey(config, example)

	// Remove LIKE escape sequences, as they are confusing in this context.
	key = likeUnescapeRewriter.Replace(key)

	return clustering.EscapeToGraphical(key)
}

// FailureAssociationRule returns a failure association rule that
// captures the definition of cluster containing the given example.
func (a *Algorithm) FailureAssociationRule(config *compiledcfg.ProjectConfig, example *clustering.Failure) string {
	if example.Reason == nil || example.Reason.PrimaryErrorMessage == "" {
		return ""
	}
	likePattern := clusterLike(config, example)

	// Escape the pattern as a double-quoted string literal suitable
	// for failure association rules.
	stringLiteral := clustering.QuoteForRule(likePattern)

	return fmt.Sprintf("reason LIKE %s", stringLiteral)
}
