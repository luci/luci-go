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

package deprecated

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/buildbucket/proto"
	annotpb "go.chromium.org/luci/common/proto/milo"
)

// This code converts annotation steps to Buildbucket v2 steps.
// See //buildbucket/proto/step.proto, //common/proto/milo/annotations.proto

// StepSep separates parent and child steps.
const StepSep = "|"

var markdownEscaper *strings.Replacer

func init() {
	const special = "#*_"
	oldNew := make([]string, 2*len(special))
	for i, c := range special {
		oldNew[i*2] = string(c)
		oldNew[i*2+1] = `\` + string(c)
	}
	markdownEscaper = strings.NewReplacer(oldNew...)
}

// ConvertBuildSteps converts a build given the root step's substeps, which must
// be the actual steps of the build, and the Logdog URL for links conversion.
// The provided context is used only for logging.
//
// Does not verify that the returned build satisfies all the constraints
// described in the proto files.
//
// Unsupported fields:
//   * Substep.annotation_stream,
//   * Step.link,
//   * Link.isolate_object,
//   * Link.dm_link.
func ConvertBuildSteps(c context.Context, annSteps []*annotpb.Step_Substep, defaultLogdogHost, defaultLogdogPrefix string) ([]*pb.Step, error) {
	sc := &stepConverter{
		defaultLogdogHost:   defaultLogdogHost,
		defaultLogdogPrefix: defaultLogdogPrefix,
		steps:               map[string]*pb.Step{},
	}

	var bbSteps []*pb.Step
	if _, err := sc.convertSubsteps(c, &bbSteps, annSteps, ""); err != nil {
		return nil, err
	}
	return bbSteps, nil
}

type stepConverter struct {
	defaultLogdogHost, defaultLogdogPrefix string

	steps map[string]*pb.Step
}

// convertSubsteps converts substeps, which we expect to be Steps, not Logdog
// annotation streams.
func (p *stepConverter) convertSubsteps(c context.Context, bbSteps *[]*pb.Step, annSubsteps []*annotpb.Step_Substep, stepPrefix string) ([]*pb.Step, error) {
	bbSubsteps := make([]*pb.Step, 0, len(annSubsteps))
	for _, annSubstep := range annSubsteps {
		annStep := annSubstep.GetStep()
		if annStep == nil {
			return nil, errors.Reason("unexpected non-Step substep %v", annSubstep).Err()
		}

		bbSubstep, err := p.convertSteps(c, bbSteps, annStep, stepPrefix)
		if err != nil {
			return nil, err
		}

		bbSubsteps = append(bbSubsteps, bbSubstep)
	}

	return bbSubsteps, nil
}

// convertSteps converts [non-root] steps.
// Mutates p.steps.
func (p *stepConverter) convertSteps(c context.Context, bbSteps *[]*pb.Step, ann *annotpb.Step, stepPrefix string) (*pb.Step, error) {
	// Set up Buildbucket v2 step.
	bb := &pb.Step{
		StartTime: ann.Started,
		EndTime:   ann.Ended,
	}

	// Annotee is known to produce startTime>endTime if they are very close.
	// Correct that here.
	if bb.StartTime != nil && bb.EndTime != nil && cmpTs(bb.StartTime, bb.EndTime) > 0 {
		if bb.StartTime.Seconds-bb.EndTime.Seconds > 1 {
			return nil, fmt.Errorf(
				"step %q start time %q is much greater than end time %q",
				ann.Name, bb.StartTime, bb.EndTime)
		}
		// Swap them. They are close enough.
		bb.StartTime, bb.EndTime = bb.EndTime, bb.StartTime
	}

	// Unlike annotation step names, buildbucket step names must be unique.
	// Choose a name.
	stripPrefix := strings.Replace(strings.TrimSuffix(stepPrefix, StepSep), "|", ".", -1) + "."
	baseName := stepPrefix + strings.TrimPrefix(ann.Name, stripPrefix)
	bb.Name = baseName
	for i := 2; p.steps[bb.Name] != nil; i++ {
		bb.Name = fmt.Sprintf("%s (%d)", baseName, i)
	}
	p.steps[bb.Name] = bb

	// summary stores a slice of paragraphs, not individual lines.
	var summary []string

	// Get any failure details.
	if fd := ann.GetFailureDetails(); fd != nil && fd.Text != "" {
		summary = append(summary, fd.Text)
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
		escaped := make([]string, len(ann.Text))
		for i, text := range ann.Text {
			escaped[i] = markdownEscaper.Replace(text)
		}
		summary = append(summary, strings.Join(escaped, " "))
	}

	// Handle logs.
	logs, lines := p.convertLinks(c, ann)
	bb.Logs = logs
	if len(lines) > 0 {
		summary = append(summary, strings.Join(lines, "\n"))
	}

	// Put completed summary into current step.
	bb.SummaryMarkdown = strings.Join(summary, "\n\n")

	// Put step into list of steps.
	*bbSteps = append(*bbSteps, bb)

	// Handle substeps.
	bbSubsteps, err := p.convertSubsteps(c, bbSteps, ann.Substep, bb.Name+StepSep)
	if err != nil {
		return nil, err
	}

	// Ensure parent step start/end time is not after/before of its children.
	for _, ss := range bbSubsteps {
		if ss.StartTime != nil && (bb.StartTime == nil || cmpTs(bb.StartTime, ss.StartTime) > 0) {
			bb.StartTime = ss.StartTime
		}
		switch {
		case ss.EndTime == nil:
			bb.EndTime = nil
		case bb.EndTime != nil && cmpTs(bb.EndTime, ss.EndTime) < 0:
			bb.EndTime = ss.EndTime
		}
	}

	// Determine status.
	switch {
	// First of all, honor start/end times.

	case bb.StartTime == nil:
		bb.Status = pb.Status_SCHEDULED

	case bb.EndTime == nil:
		bb.Status = pb.Status_STARTED

	// Secondly, honor current status.

	case ann.Status == annotpb.Status_SUCCESS:
		bb.Status = pb.Status_SUCCESS

	case ann.Status == annotpb.Status_FAILURE:
		if fd := ann.GetFailureDetails(); fd != nil {
			switch fd.Type {
			case annotpb.FailureDetails_GENERAL:
				bb.Status = pb.Status_FAILURE
			case annotpb.FailureDetails_CANCELLED:
				bb.Status = pb.Status_CANCELED
			default:
				bb.Status = pb.Status_INFRA_FAILURE
			}
		} else {
			bb.Status = pb.Status_FAILURE
		}

	default:
		return nil, errors.Reason("step %q has end time, but status is not terminal", bb.Name).Err()
	}

	// When parent step finishes running, compute its final status as worst
	// status, as determined by statusPrecedence map below, among direct children
	// and its own status.
	if protoutil.IsEnded(bb.Status) {
		for _, bbSubstep := range bbSubsteps {
			substepStatusPrecedence, ok := statusPrecedence[bbSubstep.Status]
			if ok && substepStatusPrecedence < statusPrecedence[bb.Status] {
				bb.Status = bbSubstep.Status
			}
		}
	}

	maybeCloneTimestamp(&bb.StartTime)
	maybeCloneTimestamp(&bb.EndTime)
	return bb, nil
}

func maybeCloneTimestamp(ts **timestamp.Timestamp) {
	if *ts != nil {
		*ts = proto.Clone(*ts).(*timestamp.Timestamp)
	}
}

func cmpTs(a, b *timestamp.Timestamp) int {
	if a.Seconds != b.Seconds {
		return int(a.Seconds - b.Seconds)
	}
	return int(a.Nanos - b.Nanos)
}

var statusPrecedence = map[pb.Status]int{
	pb.Status_CANCELED:      0,
	pb.Status_INFRA_FAILURE: 1,
	pb.Status_FAILURE:       2,
	pb.Status_SUCCESS:       3,
}

func (p *stepConverter) convertLinks(c context.Context, ann *annotpb.Step) ([]*pb.Step_Log, []string) {
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
	bbLogs := make([]*pb.Step_Log, 0, len(aLinks))
	summary := make([]string, 0, len(aLinks))
	logNames := stringset.New(len(aLinks))
	for _, l := range aLinks {
		switch {
		case l.GetLogdogStream() != nil:
			if logNames.Has(l.Label) {
				logging.Warningf(c, "step %q: duplicate log name %q", ann.Name, l.Label)
			} else {
				logNames.Add(l.Label)
				bbLogs = append(bbLogs,
					&pb.Step_Log{
						Name:    l.Label,
						ViewUrl: p.convertLogdogLink(l.GetLogdogStream(), true),
						Url:     p.convertLogdogLink(l.GetLogdogStream(), false),
					},
				)
			}
		case l.GetUrl() != "":
			// Arbitrary links go into the summary.
			s := l.GetUrl() // Backslash escape all parens.
			s = strings.Replace(s, `(`, `\(`, -1)
			s = strings.Replace(s, `)`, `\)`, -1)
			summary = append(summary, fmt.Sprintf("* [%s](%s)", l.Label, s))
		default:
			logging.Warningf(c, "Got neither URL nor Logdog stream, skipping: %v", l)
		}
	}

	if len(bbLogs) == 0 {
		return nil, summary
	}
	return bbLogs, summary
}

func (p *stepConverter) convertLogdogLink(log *annotpb.LogdogStream, viewURL bool) string {
	host, prefix := p.defaultLogdogHost, p.defaultLogdogPrefix
	if log.GetServer() != "" {
		host = log.Server
	}
	if log.GetPrefix() != "" {
		prefix = log.Prefix
	}
	path := fmt.Sprintf("%s/+/%s", prefix, log.Name)
	if viewURL {
		return fmt.Sprintf("https://%s/v/?s=%s", host, url.QueryEscape(path))
	}
	return fmt.Sprintf("logdog://%s/%s", host, path)
}
