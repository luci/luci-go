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

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	annotpb "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/common/types"
)

// This code converts annotation steps to Buildbucket v2 steps.
// See //buildbucket/proto/step.proto, //common/proto/milo/annotations.proto

const StepSep = "|"

// ConvertBuildSteps converts a build given the root step's substeps, which must
// be the actual steps of the build, and the Logdog URL for links conversion.
// The provided context is used only for logging.
//
// Unsupported fields:
//   * Substep.annotation_stream,
//   * Step.link,
//   * Link.isolate_object,
//   * Link.dm_link.
func ConvertBuildSteps(c context.Context, annSteps []*annotpb.Step_Substep, annURL string) ([]*buildbucketpb.Step, error) {
	sc, err := stepConverterFromURL(annURL)
	if err != nil {
		return nil, err
	}

	var bbSteps []*buildbucketpb.Step
	if err = sc.convertSubsteps(c, &bbSteps, annSteps, ""); err != nil {
		return nil, err
	}
	return bbSteps, nil
}

// stepConverterFromURL returns stepConverter given an annotation URL.
func stepConverterFromURL(annURL string) (*stepConverter, error) {
	addr, err := types.ParseURL(annURL)
	if err != nil {
		return nil, errors.Annotate(err, "could not parse LogDog stream from annotation URL").Err()
	}

	prefix, _ := addr.Path.Split()
	return &stepConverter{
		defaultLogdogHost:   addr.Host,
		defaultLogdogPrefix: fmt.Sprintf("%s/%s", addr.Project, prefix),
	}, nil
}

type stepConverter struct {
	defaultLogdogHost, defaultLogdogPrefix string
}

// convertSubsteps converts substeps, which we expect to be Steps, not Logdog
// annotation streams.
func (p *stepConverter) convertSubsteps(c context.Context, bbSteps *[]*buildbucketpb.Step, annSubsteps []*annotpb.Step_Substep, stepPrefix string) error {
	for _, annSubstep := range annSubsteps {
		annStep := annSubstep.GetStep()
		if annStep == nil {
			return errors.Reason("unexpected non-Step substep %v", annSubstep).Err()
		}

		err := p.convertSteps(c, bbSteps, annStep, stepPrefix)
		if err != nil {
			return err
		}
	}

	return nil
}

// convertSteps converts [non-root] steps.
func (p *stepConverter) convertSteps(c context.Context, bbSteps *[]*buildbucketpb.Step, ann *annotpb.Step, stepPrefix string) error {
	// Set up Buildbucket v2 step.
	bb := &buildbucketpb.Step{
		Name:      stepPrefix + ann.Name,
		Status:    p.convertStatus(ann),
		StartTime: ann.Started,
		EndTime:   ann.Ended,
	}

	// summary stores a slice of paragraphs, not individual lines.
	var summary []string

	// Get any failure details.
	if fd := ann.GetFailureDetails(); fd != nil && fd.Text != "" {
		summary = append(summary, fd.Text)
	}

	// Handle logs.
	logs, lines := p.convertLinks(c, ann)
	bb.Logs = logs
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

	// Put completed summary into current step.
	bb.SummaryMarkdown = strings.Join(summary, "\n\n")

	// Put step into list of steps.
	*bbSteps = append(*bbSteps, bb)

	// Handle substeps.
	return p.convertSubsteps(c, bbSteps, ann.Substep, bb.Name+StepSep)
}

func (p *stepConverter) convertStatus(ann *annotpb.Step) buildbucketpb.Status {
	switch ann.Status {
	case annotpb.Status_RUNNING:
		return buildbucketpb.Status_STARTED

	case annotpb.Status_SUCCESS:
		return buildbucketpb.Status_SUCCESS

	case annotpb.Status_FAILURE:
		if fd := ann.GetFailureDetails(); fd != nil {
			switch fd.Type {
			case annotpb.FailureDetails_GENERAL:
				return buildbucketpb.Status_FAILURE
			case annotpb.FailureDetails_CANCELLED:
				return buildbucketpb.Status_CANCELED
			default:
				return buildbucketpb.Status_INFRA_FAILURE
			}
		}
		return buildbucketpb.Status_FAILURE
	}

	return buildbucketpb.Status_STATUS_UNSPECIFIED
}

func (p *stepConverter) convertLinks(c context.Context, ann *annotpb.Step) ([]*buildbucketpb.Step_Log, []string) {
	aLinks := make([]*annotpb.Link, 0, len(ann.OtherLinks)+2)

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

	// Convert each link to a Buildbucket v2 log.
	bbLogs := make([]*buildbucketpb.Step_Log, 0, len(aLinks))
	summary := make([]string, 0, len(aLinks))
	for _, l := range aLinks {
		switch {
		case l.GetLogdogStream() != nil:
			bbLogs = append(bbLogs,
				&buildbucketpb.Step_Log{
					Name:    l.Label,
					ViewUrl: p.convertLogdogLink(l.GetLogdogStream()),
				},
			)
		case l.GetUrl() != "":
			// Arbitrary links go into the summary.
			summary = append(summary, fmt.Sprintf("* [%s](%s)", l.Label, l.GetUrl()))
		default:
			logging.Warningf(c, "Got neither URL nor Logdog stream, skipping: %v", l)
		}
	}

	if len(bbLogs) == 0 {
		return nil, summary
	}
	return bbLogs, summary
}

func (p *stepConverter) convertLogdogLink(log *annotpb.LogdogStream) string {
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
