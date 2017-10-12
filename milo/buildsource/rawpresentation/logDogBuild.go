// Copyright 2016 The LUCI Authors.
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

package rawpresentation

import (
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/common/model"
)

// URLBuilder constructs URLs for various link types.
type URLBuilder interface {
	// LinkURL returns the URL associated with the supplied Link.
	//
	// If no URL could be built for that Link, nil will be returned.
	BuildLink(l *miloProto.Link) *resp.Link
}

// HACK(hinoka): This should be a part of recipes, but just hardcoding a list
// of unimportant things for now.
var builtIn = map[string]struct{}{
	"recipe bootstrap": {},
	"setup_build":      {},
	"recipe result":    {},
}

// miloBuildStep converts a logdog/milo step to a BuildComponent struct.
// buildCompletedTime must be zero if build did not complete yet.
func miloBuildStep(ub URLBuilder, anno *miloProto.Step, isMain bool, buildCompletedTime,
	now time.Time) []*resp.BuildComponent {

	comp := &resp.BuildComponent{Label: anno.Name}
	switch anno.Status {
	case miloProto.Status_RUNNING:
		comp.Status = model.Running

	case miloProto.Status_SUCCESS:
		comp.Status = model.Success

	case miloProto.Status_FAILURE:
		if fd := anno.GetFailureDetails(); fd != nil {
			switch fd.Type {
			case miloProto.FailureDetails_EXCEPTION, miloProto.FailureDetails_INFRA:
				comp.Status = model.InfraFailure

			case miloProto.FailureDetails_EXPIRED:
				comp.Status = model.Expired

			default:
				comp.Status = model.Failure
			}

			if fd.Text != "" {
				comp.Text = append(comp.Text, fd.Text)
			}
		} else {
			comp.Status = model.Failure
		}

	case miloProto.Status_PENDING:
		comp.Status = model.NotRun

		// Missing the case of waiting on unfinished dependency...
	default:
		comp.Status = model.NotRun
	}

	if !(buildCompletedTime.IsZero() || comp.Status.Terminal()) {
		// The build has completed, but this step has not. Mark it as an
		// infrastructure failure.
		comp.Status = model.InfraFailure
	}

	// Hide the unimportant steps, highlight the interesting ones.
	switch comp.Status {
	case model.NotRun, model.Running:
		if isMain {
			comp.Verbosity = resp.Hidden
		}

	case model.Success:
		if _, ok := builtIn[anno.Name]; ok || isMain {
			comp.Verbosity = resp.Hidden
		}
	case model.InfraFailure, model.Failure:
		comp.Verbosity = resp.Interesting
	}

	// Main link is a link to the stdout.
	var stdoutLink *miloProto.Link
	if anno.StdoutStream != nil {
		stdoutLink = &miloProto.Link{
			Label: "stdout",
			Value: &miloProto.Link_LogdogStream{
				LogdogStream: anno.StdoutStream,
			},
		}
	}

	if anno.Link != nil {
		comp.MainLink = resp.LinkSet{ub.BuildLink(anno.Link)}

		// If we also have a STDOUT stream, add it to our OtherLinks.
		if stdoutLink != nil {
			anno.OtherLinks = append([]*miloProto.Link{stdoutLink}, anno.OtherLinks...)
		}
	} else if stdoutLink != nil {
		comp.MainLink = resp.LinkSet{ub.BuildLink(stdoutLink)}
	}

	// Add STDERR link, if available.
	if anno.StderrStream != nil {
		anno.OtherLinks = append(anno.OtherLinks, &miloProto.Link{
			Label: "stderr",
			Value: &miloProto.Link_LogdogStream{
				LogdogStream: anno.StderrStream,
			},
		})
	}

	// Sub link is for one link per log that isn't stdout.
	for _, link := range anno.GetOtherLinks() {
		if l := ub.BuildLink(link); l != nil {
			comp.SubLink = append(comp.SubLink, resp.LinkSet{l})
		}
	}

	// This should always be a step.
	comp.Type = resp.Step

	// This should always be 0
	comp.LevelsDeep = 0

	// Timestamps
	comp.Started = google.TimeFromProto(anno.Started)
	comp.Finished = google.TimeFromProto(anno.Ended)

	var till time.Time
	switch {
	case !comp.Finished.IsZero():
		till = comp.Finished
	case comp.Status == model.Running:
		till = now
	case !buildCompletedTime.IsZero():
		till = buildCompletedTime
	}
	if !comp.Started.IsZero() && till.After(comp.Started) {
		comp.Duration = till.Sub(comp.Started)
	}

	// This should be the exact same thing.
	comp.Text = append(comp.Text, anno.Text...)

	ss := anno.GetSubstep()
	results := []*resp.BuildComponent{}
	results = append(results, comp)
	// Process nested steps.
	for _, substep := range ss {
		var subanno *miloProto.Step
		switch s := substep.GetSubstep().(type) {
		case *miloProto.Step_Substep_Step:
			subanno = s.Step
		case *miloProto.Step_Substep_AnnotationStream:
			panic("Non-inline substeps not supported")
		default:
			panic(fmt.Errorf("Unknown type %v", s))
		}
		for _, subcomp := range miloBuildStep(ub, subanno, false, buildCompletedTime, now) {
			results = append(results, subcomp)
		}
	}

	return results
}

// AddLogDogToBuild takes a set of logdog streams and populate a milo build.
// build.Summary.Finished must be set.
func AddLogDogToBuild(
	c context.Context, ub URLBuilder, mainAnno *miloProto.Step, build *resp.MiloBuild) {
	now := clock.Now(c)

	// Now fill in each of the step components.
	// TODO(hinoka): This is totes cachable.
	buildCompletedTime := google.TimeFromProto(mainAnno.Ended)
	build.Summary = *(miloBuildStep(ub, mainAnno, true, buildCompletedTime, now)[0])
	propMap := map[string]string{}
	for _, substepContainer := range mainAnno.Substep {
		anno := substepContainer.GetStep()
		if anno == nil {
			// TODO: We ignore non-embedded substeps for now.
			continue
		}

		bss := miloBuildStep(ub, anno, false, buildCompletedTime, now)
		for _, bs := range bss {
			if bs.Status != model.Success {
				build.Summary.Text = append(
					build.Summary.Text, fmt.Sprintf("%s %s", bs.Status, bs.Label))
			}
			build.Components = append(build.Components, bs)
			propGroup := &resp.PropertyGroup{GroupName: bs.Label}
			for _, prop := range anno.Property {
				propGroup.Property = append(propGroup.Property, &resp.Property{
					Key:   prop.Name,
					Value: prop.Value,
				})
				propMap[prop.Name] = prop.Value
			}
			build.PropertyGroup = append(build.PropertyGroup, propGroup)
		}
	}

	// Take care of properties
	propGroup := &resp.PropertyGroup{GroupName: "Main"}
	for _, prop := range mainAnno.Property {
		propGroup.Property = append(propGroup.Property, &resp.Property{
			Key:   prop.Name,
			Value: prop.Value,
		})
		propMap[prop.Name] = prop.Value
	}
	build.PropertyGroup = append(build.PropertyGroup, propGroup)

	// HACK(hinoka,iannucci): Extract revision out of properties. This should use
	// source manifests instead.
	jrev, ok := propMap["got_revision"]
	if !ok {
		jrev, ok = propMap["revision"]
	}
	if ok {
		// got_revision/revision are json strings, so it looks like "aaaaaabbcc123..."
		var rev string
		err := json.Unmarshal([]byte(jrev), &rev)
		if err == nil {
			if build.Trigger == nil {
				build.Trigger = &resp.Trigger{}
			}
			build.Trigger.Revision = resp.NewLink(
				rev, fmt.Sprintf("https://crrev.com/%s", rev))
		}
	}

	return
}
