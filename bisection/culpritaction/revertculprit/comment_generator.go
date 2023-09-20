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
	"strings"
	"text/template"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/common/errors"
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
	analysisURL := util.ConstructAnalysisURL(ctx, bbid)
	buildURL := util.ConstructBuildURL(ctx, bbid)
	bugURL := util.ConstructLUCIBisectionBugURL(ctx, analysisURL, suspect.ReviewUrl)
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
{{ define "basic"}}
Sample build with failed test: {{.BuildURL}}
Affected test(s):
{{.TestLinks}}

If this is a false positive, please report it at {{.BugURL}}
{{- end}}

{{ define "supportComment" -}}
LUCI Bisection recommends submitting this revert because it has confirmed the target of this revert is the cause of a test failure.
{{ template "basic" . -}}
{{end}}

{{ define "blameComment" -}}
LUCI Bisection has identified this change as the cause of a test failure.
{{ template "basic" . -}}
{{end}}
	`))

func testFailureComment(ctx context.Context, suspect *model.Suspect, reason, templateName string) (string, error) {
	tfs, err := datastoreutil.FetchTestFailuresForSuspect(ctx, suspect)
	if err != nil {
		return "", errors.Annotate(err, "fetch test failure for suspect").Err()
	}
	bbid, err := datastoreutil.GetAssociatedBuildID(ctx, suspect)
	if err != nil {
		return "", err
	}
	buildURL := util.ConstructBuildURL(ctx, bbid)
	testLinks := []string{}
	for _, tf := range tfs.NonDiverged() {
		testLinks = append(testLinks, fmt.Sprintf("(%s)[%s]", tf.TestID, util.ConstructTestHistoryURL(tf.Project, tf.TestID, tf.VariantHash)))
	}
	bugURL := util.ConstructBuganizerURLForTestAnalysis(suspect.ReviewUrl, tfs.Primary().AnalysisKey.IntID())
	// TODO(beining@): add analysis URL when the bisection UI is ready.
	var b bytes.Buffer
	err = testCommentTemplate.ExecuteTemplate(&b, templateName, map[string]any{
		"TestLinks": strings.Join(testLinks, "\n"),
		"BugURL":    bugURL,
		"BuildURL":  buildURL,
		"Reason":    reason,
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}
