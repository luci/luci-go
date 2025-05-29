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

package annotee

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	annotpb "go.chromium.org/luci/luciexe/legacy/annotee/proto"
)

// This code converts annotation step(s) to Buildbucket v2 steps or build.
// See:
//   * go.chromium.org/luci/buildbucket/proto#Build
//   * go.chromium.org/luci/buildbucket/proto#Step
//   * go.chromium.org/luci/luciexe/legacy/annotee/proto#Step

// StepSep separates parent and child steps.
const StepSep = "|"

// ConvertBuildSteps converts a build given the root step's substeps, which must
// be the actual steps of the build, and the Logdog URL for links conversion.
// The provided context is used only for logging.
//
// If constructLogURL is True, all returned steps will have fully-qualified
// logdog urls (i.e. logdog://host/prefix/+/log_name) and viewer urls will also
// be populated. Otherwise, the logdog url will simply be the same as log
// stream name and it is caller's responsibility to calculate the full URL.
// Values of defaultLogdogHost and defaultLogdogPrefix are only meaningful when
// constructLogURL is True.
// TODO(yiwzhang): Remove the option to construct full logdog url after kitchen
// is deprecated. Currently, this functionality is used in Kitchen.
//
// Does not verify that the returned build satisfies all the constraints
// described in the proto files.
//
// Unsupported fields:
//   - Substep.annotation_stream,
//   - Step.link,
//   - Link.isolate_object,
//   - Link.dm_link.
func ConvertBuildSteps(c context.Context, annSteps []*annotpb.Step_Substep, constructLogURL bool, defaultLogdogHost, defaultLogdogPrefix string) ([]*pb.Step, error) {
	sc := &stepConverter{
		defaultLogdogHost:   defaultLogdogHost,
		defaultLogdogPrefix: defaultLogdogPrefix,
		constructLogURL:     constructLogURL,
		steps:               map[string]*pb.Step{},
	}

	var bbSteps []*pb.Step
	if _, err := sc.convertSubsteps(c, &bbSteps, annSteps, ""); err != nil {
		return nil, err
	}
	return bbSteps, nil
}

// ConvertRootStep converts an annotation root step to a Build proto msg.
//
// This function will populate following fields in the return build.
//   - EndTime
//   - StartTime
//   - Steps
//   - Status
//   - SummaryMarkdown
//   - Output.Logs
//   - Output.Properties
func ConvertRootStep(c context.Context, rootStep *annotpb.Step) (*pb.Build, error) {
	sc := &stepConverter{
		steps: map[string]*pb.Step{},
	}
	ret := &pb.Build{
		StartTime: rootStep.Started,
		EndTime:   rootStep.Ended,
		Steps:     []*pb.Step{},
		Output:    &pb.Build_Output{},
	}
	var err error

	if _, err = sc.convertSubsteps(c, &ret.Steps, rootStep.Substep, ""); err != nil {
		return nil, err
	}

	if ret.Output.Properties, err = annotpb.ExtractProperties(rootStep); err != nil {
		return nil, err
	}

	if ret.StartTime, ret.EndTime, err = fixupStartAndEndTime(ret.StartTime, ret.EndTime, ret.Steps); err != nil {
		return nil, err
	}

	if ret.Status, err = determineStatus(ret.StartTime, ret.EndTime, rootStep, ret.Steps); err != nil {
		return nil, err
	}

	ret.Output.Logs, ret.SummaryMarkdown = sc.calcLogsAndSummary(c, rootStep)

	ret.Output.Status = ret.Status
	ret.Output.SummaryMarkdown = ret.SummaryMarkdown

	return ret, nil
}

type stepConverter struct {
	defaultLogdogHost, defaultLogdogPrefix string
	constructLogURL                        bool
	steps                                  map[string]*pb.Step
}

// convertSubsteps converts substeps, which we expect to be Steps, not Logdog
// annotation streams.
func (p *stepConverter) convertSubsteps(c context.Context, bbSteps *[]*pb.Step, annSubsteps []*annotpb.Step_Substep, stepPrefix string) ([]*pb.Step, error) {
	bbSubsteps := make([]*pb.Step, 0, len(annSubsteps))
	for _, annSubstep := range annSubsteps {
		annStep := annSubstep.GetStep()
		if annStep == nil {
			return nil, errors.Fmt("unexpected non-Step substep %v", annSubstep)
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

	// Unlike annotation step names, buildbucket step names must be unique.
	// Choose a name.
	stripPrefix := strings.Replace(strings.TrimSuffix(stepPrefix, StepSep), "|", ".", -1) + "."
	baseName := stepPrefix + strings.TrimPrefix(ann.Name, stripPrefix)
	bb.Name = baseName
	for i := 2; p.steps[bb.Name] != nil; i++ {
		bb.Name = fmt.Sprintf("%s (%d)", baseName, i)
	}
	p.steps[bb.Name] = bb

	bb.Logs, bb.SummaryMarkdown = p.calcLogsAndSummary(c, ann)

	// Put step into list of steps.
	*bbSteps = append(*bbSteps, bb)

	// Handle substeps.
	bbSubsteps, err := p.convertSubsteps(c, bbSteps, ann.Substep, bb.Name+StepSep)
	if err != nil {
		return nil, err
	}

	if bb.StartTime, bb.EndTime, err = fixupStartAndEndTime(bb.StartTime, bb.EndTime, bbSubsteps); err != nil {
		return nil, errors.Fmt("Step %s: : %w", bb.Name, err)
	}

	if bb.Status, err = determineStatus(bb.StartTime, bb.EndTime, ann, bbSubsteps); err != nil {
		return nil, errors.Fmt("Step %s: : %w", bb.Name, err)
	}

	maybeCloneTimestamp(&bb.StartTime)
	maybeCloneTimestamp(&bb.EndTime)
	return bb, nil
}

func (p *stepConverter) calcLogsAndSummary(ctx context.Context, annStep *annotpb.Step) ([]*pb.Log, string) {
	// summary stores a slice of paragraphs, not individual lines.
	var summary []string

	// Get any failure details.
	if fd := annStep.GetFailureDetails(); fd != nil && fd.Text != "" {
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
	if len(annStep.Text) > 0 {
		// enclose ann.Text in a div so they are treated as plain text or HTML.
		// use 2 \n so the div is guaranteed to be separated from other sections
		summary = append(summary, fmt.Sprintf("\n\n<div>%s</div>\n\n", strings.Join(annStep.Text, " ")))
	}

	// Handle logs.
	logs, lines := p.convertLinks(ctx, annStep)

	if len(lines) > 0 {
		summary = append(summary, strings.Join(lines, "\n"))
	}

	return logs, strings.Join(summary, "\n\n")
}

func fixupStartAndEndTime(startTime, endTime *timestamppb.Timestamp, subSteps []*pb.Step) (newStart, newEnd *timestamppb.Timestamp, err error) {
	newStart, newEnd = startTime, endTime

	// Annotee is known to produce startTime>endTime if they are very close.
	// Correct that here.
	if startTime != nil && endTime != nil && cmpTs(startTime, endTime) > 0 {
		if startTime.Seconds-endTime.Seconds > 1 {
			return nil, nil, errors.Fmt("start time %q is much greater than end time %q", startTime, endTime)
		}
		// Swap them. They are close enough.
		newStart, newEnd = endTime, startTime
	}

	// Ensure parent step start/end time is not after/before of its children.
	for _, ss := range subSteps {
		if ss.StartTime != nil && (newStart == nil || cmpTs(newStart, ss.StartTime) > 0) {
			newStart = ss.StartTime
		}
		switch {
		case ss.EndTime == nil:
			newEnd = nil
		case newEnd != nil && cmpTs(newEnd, ss.EndTime) < 0:
			newEnd = ss.EndTime
		}
	}
	return
}

func determineStatus(startTime, endTime *timestamppb.Timestamp, annStep *annotpb.Step, subSteps []*pb.Step) (ret pb.Status, err error) {
	// Determine status.
	switch {
	// First of all, honor start/end times.
	case startTime == nil:
		ret = pb.Status_SCHEDULED

	case endTime == nil:
		ret = pb.Status_STARTED

	// Secondly, honor current status.
	case annStep.Status == annotpb.Status_SUCCESS:
		ret = pb.Status_SUCCESS

	case annStep.Status == annotpb.Status_FAILURE:
		if fd := annStep.GetFailureDetails(); fd != nil {
			switch fd.Type {
			case annotpb.FailureDetails_GENERAL:
				ret = pb.Status_FAILURE
			case annotpb.FailureDetails_CANCELLED:
				ret = pb.Status_CANCELED
			default:
				ret = pb.Status_INFRA_FAILURE
			}
		} else {
			ret = pb.Status_FAILURE
		}

	default:
		return ret, errors.Fmt("expected terminal status when endTime is not nil, got %s", annStep.Status)
	}

	// When parent step finishes running, compute its final status as worst
	// status, as determined by statusPrecedence map below, among direct children
	// and its own status.
	if protoutil.IsEnded(ret) {
		for _, subStep := range subSteps {
			substepStatusPrecedence, ok := statusPrecedence[subStep.Status]
			if ok && substepStatusPrecedence < statusPrecedence[ret] {
				ret = subStep.Status
			}
		}
	}

	return
}

func maybeCloneTimestamp(ts **timestamppb.Timestamp) {
	if *ts != nil {
		*ts = proto.Clone(*ts).(*timestamppb.Timestamp)
	}
}

func cmpTs(a, b *timestamppb.Timestamp) int {
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

func (p *stepConverter) convertLinks(c context.Context, ann *annotpb.Step) ([]*pb.Log, []string) {
	aLinks := make([]*annotpb.AnnotationLink, 0, len(ann.OtherLinks)+2)

	// Get stdout, stderr Logdog links.
	if ann.StdoutStream != nil {
		aLinks = append(aLinks,
			&annotpb.AnnotationLink{
				Label: "stdout",
				Value: &annotpb.AnnotationLink_LogdogStream{LogdogStream: ann.StdoutStream},
			},
		)
	}
	if ann.StderrStream != nil {
		aLinks = append(aLinks,
			&annotpb.AnnotationLink{
				Label: "stderr",
				Value: &annotpb.AnnotationLink_LogdogStream{LogdogStream: ann.StderrStream},
			},
		)
	}

	// Get "other" links (ann.Link is expected never to be initialized).
	if ann.GetLink() != nil {
		logging.Warningf(c, "Got unexpectedly populated link field in annotation step %v", ann)
	}
	aLinks = append(aLinks, ann.OtherLinks...)

	// Convert each link to a Buildbucket v2 log.
	bbLogs := make([]*pb.Log, 0, len(aLinks))
	summary := make([]string, 0, len(aLinks))
	logNames := stringset.New(len(aLinks))
	for _, l := range aLinks {
		switch {
		case l.GetLogdogStream() != nil:
			if logNames.Has(l.Label) {
				logging.Warningf(c, "step %q: duplicate log name %q", ann.Name, l.Label)
			} else {
				logNames.Add(l.Label)
				bbLog := &pb.Log{
					Name: l.Label,
					Url:  l.GetLogdogStream().Name,
				}
				if p.constructLogURL {
					bbLog.ViewUrl = p.constructLogdogLink(l.GetLogdogStream(), true)
					bbLog.Url = p.constructLogdogLink(l.GetLogdogStream(), false)
				}
				bbLogs = append(bbLogs, bbLog)
			}
		case l.GetUrl() != "":
			// Arbitrary links go into the summary.
			s := l.GetUrl() // Backslash escape all parens.
			s = strings.Replace(s, `(`, `\(`, -1)
			s = strings.Replace(s, `)`, `\)`, -1)
			s = strings.Replace(s, `&`, `&amp;`, -1)
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

func (p *stepConverter) constructLogdogLink(log *annotpb.LogdogStream, viewURL bool) string {
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
