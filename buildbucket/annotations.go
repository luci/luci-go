// Copyright 2018 The LUCI Authors.
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

package buildbucket

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	annotpb "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/common/types"
)

// This code converts annotation steps to Buildbucket v2 steps.
// See build/result.proto, buildbucket/proto/step.proto, common/proto/milo/annotations.proto

var STEP_SEP = "|"

// Given an annotation URL as from BuildRunResult, get stepParser.
func stepParserFromUrl(annUrl string) (*stepParser, error) {
	addr, err := types.ParseURL(annUrl)
	if err != nil {
		return nil, errors.Annotate(err, "could not parse LogDog stream from annotation URL").Err()
	}

	prefix, _ := addr.Path.Split()
	return &stepParser{
		defaultLogdogHost: addr.Host,
		defaultLogdogPrefix: fmt.Sprintf("%s/%s", addr.Project, prefix),
	}, nil
}

type stepParser struct {
	defaultLogdogHost, defaultLogdogPrefix string
}

// Handle root step, which contains only substeps, which are the actual steps of the build.
// This is the only function that should be used to convert annotation steps to Buildbucket steps.
func (p *stepParser) handleBuildSteps(c context.Context, ann *annotpb.Step) ([]*bbpb.Step, error) {
	return p.handleSubsteps(c, ann, "")
}

// Thin wrapper for handling substeps, which we expect to be Steps, not Logdog annotation streams.
func (p *stepParser) handleSubsteps(c context.Context, ann *annotpb.Step, stepPrefix string) ([]*bbpb.Step, error) {
	bbSteps := []*bbpb.Step{}
	var annSubstep *annotpb.Step
	var bbSubsteps []*bbpb.Step
	var err error
	for _, annStep := range ann.Substep {
		annSubstep = annStep.GetStep()
		if annSubstep == nil {
			return []*bbpb.Step{}, errors.New(fmt.Sprintf("unexpected non-Step substep %v", annStep))
		}

		bbSubsteps, err = p.handleSteps(c, annSubstep, stepPrefix)
		if err != nil {
			return []*bbpb.Step{}, err
		}

		bbSteps = append(bbSteps, bbSubsteps...)
	}

	return bbSteps, nil
}

// Handle steps (meant for non-root use).
func (p *stepParser) handleSteps(c context.Context, ann *annotpb.Step, stepPrefix string) ([]*bbpb.Step, error) {
	// Set up Buildbucket v2 step.
	bb := &bbpb.Step{
		Name:      stepPrefix + ann.Name,
		Status:    p.handleStatus(ann),
	}
	if ann.Started != nil {
		bb.StartTime = ann.Started
	}
	if ann.Ended != nil {
		bb.EndTime = ann.Ended
	}

	summary := []string{}

	// Get any failure details.
	if fd := ann.GetFailureDetails(); fd != nil && fd.Text != "" {
		summary = append(summary, fd.Text)
	}

	// Handle links.
	links, lines := p.handleLinks(c, ann)
	bb.Logs = links
	if len(lines) > 0 {
		summary = append(summary, strings.Join(lines, "\n"))
	}

	// Handle text. Below description copied from
	// https://cs.chromium.org/chromium/infra/appengine/cr-buildbucket/annotations.py?l=85&rcl=ec46b5a76fd9948d43a9116051435cbccafa12f1.
	// Although annotation.proto says each line in step_text is a consecutive
	// line and should not contain newlines, in practice they are in HTML format
	// may have <br>s, Buildbot joins them with " " and treats the result
	// as safe HTML.
	// https://cs.chromium.org/chromium/build/third_party/buildbot_8_4p1/buildbot/status/web/build.py?sq=package:chromium&rcl=83e20043dedd1db6977c6aa818e66c1f82ff31e1&l=130
	// Preserve this semantics (except, it is not safe).
	// HTML is valid Markdown, so use it as is.
	if len(ann.Text) > 0 {
		summary = append(summary, strings.Join(ann.Text, " "))
	}

	// Put step into list of steps.
	bbSteps := []*bbpb.Step{bb}

	// Handle substeps.
	// We expect only Step substeps, because logdog only generates annotation Step protos with Step
	// substeps. If this changes, we don't want to ignore silently, so log an error in case of getting
	// a Logdog annotation stream as substep..
	prefix := bb.Name + STEP_SEP
	substeps, err := p.handleSubsteps(c, ann, prefix)
	if err != nil {
		return []*bbpb.Step{}, err
	}
	bbSteps = append(bbSteps, substeps...)

	// Put completed summary into current step.
	bb.SummaryMarkdown = strings.Join(summary, "\n\n")
	return bbSteps, nil
}

func (p *stepParser) handleStatus(ann *annotpb.Step) bbpb.Status {
	switch ann.Status {
	case annotpb.Status_RUNNING:
		return bbpb.Status_STARTED

	case annotpb.Status_SUCCESS:
		return bbpb.Status_SUCCESS

	case annotpb.Status_FAILURE:
		if fd := ann.GetFailureDetails(); fd != nil {
			switch fd.Type {
			case annotpb.FailureDetails_GENERAL:
				return bbpb.Status_FAILURE
			case annotpb.FailureDetails_CANCELLED:
				return bbpb.Status_CANCELED
			default:
				return bbpb.Status_INFRA_FAILURE
			}
		}
		return bbpb.Status_FAILURE
	}

	return bbpb.Status_STATUS_UNSPECIFIED
}

func (p *stepParser) handleLinks(c context.Context, ann *annotpb.Step) ([]*bbpb.Step_Log, []string) {
	aLinks := []*annotpb.Link{}

	// Get stdout, stderr Logdog links.
	if ann.StdoutStream != nil {
		aLinks = append(aLinks,
			&annotpb.Link{
				Label: "stdout",
				Value: &annotpb.Link_LogdogStream{ann.StdoutStream},
			},
		)
	}
	if ann.StderrStream != nil {
		aLinks = append(aLinks,
			&annotpb.Link{
				Label: "stderr",
				Value: &annotpb.Link_LogdogStream{ann.StderrStream},
			},
		)
	}

	// Get "other" links (ann.Link is expected never to be initialized).
	if ann.GetLink() != nil {
		logging.Warningf(c, "Got unexpectedly populated link field in annotation step %v", ann)
	}
	aLinks = append(aLinks, ann.OtherLinks...)

	// Convert each link to a Buildbucket v2 link.
	bbLinks := make([]*bbpb.Step_Log, 0, len(aLinks))
	summary := []string{}
	for _, l := range aLinks {
		switch {
		case l.GetLogdogStream() != nil:
			bbLinks = append(bbLinks,
				&bbpb.Step_Log{
					Name:    l.Label,
					ViewUrl: p.handleLogdogLink(l.GetLogdogStream()),
				},
			)
		case l.GetUrl() != "":
			// Arbitrary links go into the summary.
			summary = append(summary, fmt.Sprintf("* [%s](%s)", l.Label, l.GetUrl()))
		default:
			logging.Warningf(c, "Got neither URL nor Logdog stream, skipping: %v", l)
		}
	}

	if len(bbLinks) == 0 {
		return nil, summary
	}
	return bbLinks, summary
}

func (p *stepParser) handleLogdogLink(log *annotpb.LogdogStream) string {
	host, prefix := p.defaultLogdogHost, p.defaultLogdogPrefix
	if log.GetServer() != "" {
		host = log.Server
	}
	if log.GetPrefix() != "" {
		prefix = log.Prefix
	}
	path := fmt.Sprintf("%s/+/%s", prefix, log.Name)
	return fmt.Sprintf("https://%s/v/?s=%s", host, url.QueryEscape(path))
}
