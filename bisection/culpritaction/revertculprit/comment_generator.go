// Copyright 2023 The LUCI Authors.
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

package revertculprit

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

var compileCommentTemplate = template.Must(template.New("").Parse(
	`
{{define "basic"}}
{{if .Reason}}
A revert for this change was not created because {{.Reason}}.
{{end}}
Sample failed build: {{.BuildURL}}

If this is a false positive, please report it at {{.BugURL}}
{{- end}}

{{define "supportComment" -}}
LUCI Bisection recommends submitting this revert because it has confirmed the target of this revert is the culprit of a build failure. See the analysis: {{.AnalysisURL}}
 {{- template "basic" . -}}
{{end}}

{{define "blameComment" -}}
LUCI Bisection has identified this change as the culprit of a build failure. See the analysis: {{.AnalysisURL}}
{{- template "basic" . -}}
{{end}}
	`))

func compileFailureComment(ctx context.Context, suspect *model.Suspect, reason, templateName string) (string, error) {
	bbid, err := datastoreutil.GetAssociatedBuildID(ctx, suspect)
	if err != nil {
		return "", err
	}
	// TODO(beining@): remove the hardcoded project name to support multiple LUCI projects.
	analysisURL := util.ConstructCompileAnalysisURL("chromium", bbid)
	buildURL := util.ConstructBuildURL(ctx, bbid)
	bugURL := util.ConstructBuganizerURLForAnalysis(analysisURL, suspect.ReviewUrl)
	var b bytes.Buffer
	err = compileCommentTemplate.ExecuteTemplate(&b, templateName, map[string]any{
		"AnalysisURL": analysisURL,
		"BugURL":      bugURL,
		"BuildURL":    buildURL,
		"Reason":      reason,
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

var testCommentTemplate = template.Must(template.New("").Parse(
	`
{{define "basic"}} See the analysis: {{.AnalysisURL}}

Sample build with failed test: {{.BuildURL}}
Affected test(s):
{{.TestLinks}}
{{- if gt .numTestLinksHidden 0}}
and {{.numTestLinksHidden}} more ...
{{- end}}
{{- if .Reason}}
A revert for this change was not created because {{.Reason}}.
{{- end}}

If this is a false positive, please report it at {{.BugURL}}
{{- end}}

{{ define "supportComment" -}}
LUCI Bisection recommends submitting this revert because it has confirmed the target of this revert is the cause of a test failure.
{{- template "basic" . -}}
{{end}}

{{ define "blameComment" -}}
LUCI Bisection has identified this change as the cause of a test failure.
{{- template "basic" . -}}
{{end}}
	`))

const maxTestLink = 5

func testFailureComment(ctx context.Context, suspect *model.Suspect, reason, templateName string) (string, error) {
	tfs, err := datastoreutil.FetchTestFailuresForSuspect(ctx, suspect)
	if err != nil {
		return "", errors.Fmt("fetch test failure for suspect: %w", err)
	}
	bbid, err := datastoreutil.GetAssociatedBuildID(ctx, suspect)
	if err != nil {
		return "", err
	}
	buildURL := util.ConstructBuildURL(ctx, bbid)
	testFailures := tfs.NonDiverged()
	sortTestFailures(testFailures)
	testLinks := []string{}
	for _, tf := range testFailures[:min(maxTestLink, len(testFailures))] {
		testLinks = append(testLinks, fmt.Sprintf("[%s](%s)", tf.TestID, util.ConstructTestHistoryURL(tf.Project, tf.TestID, tf.VariantHash)))
	}
	analysisID := tfs.Primary().AnalysisKey.IntID()
	// TODO(beining@): remove the hardcoded project name to support multiple LUCI projects.
	analysisURL := util.ConstructTestAnalysisURL("chromium", analysisID)
	bugURL := util.ConstructBuganizerURLForAnalysis(suspect.ReviewUrl, analysisURL)
	var b bytes.Buffer
	err = testCommentTemplate.ExecuteTemplate(&b, templateName, map[string]any{
		"AnalysisURL":        analysisURL,
		"TestLinks":          strings.Join(testLinks, "\n"),
		"numTestLinksHidden": len(testFailures) - maxTestLink,
		"BugURL":             bugURL,
		"BuildURL":           buildURL,
		"Reason":             reason,
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func sortTestFailures(tfs []*model.TestFailure) {
	sort.Slice(tfs, func(i, j int) bool {
		if tfs[i].TestID == tfs[j].TestID {
			return tfs[i].VariantHash < tfs[j].VariantHash
		}
		return tfs[i].TestID < tfs[j].TestID
	})
}
