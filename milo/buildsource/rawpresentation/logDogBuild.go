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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"

	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// URLBuilder constructs URLs for various link types.
type URLBuilder interface {
	// LinkURL returns the URL associated with the supplied Link.
	//
	// If no URL could be built for that Link, nil will be returned.
	BuildLink(l *miloProto.Link) *ui.Link
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
func miloBuildStep(c context.Context, ub URLBuilder, anno *miloProto.Step) []*ui.BuildComponent {

	comp := &ui.BuildComponent{Label: ui.NewLink(anno.Name, "", anno.Name)}
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

	// Hide the unimportant steps, highlight the interesting ones.
	switch comp.Status {
	case model.Success:
		if _, ok := builtIn[anno.Name]; ok {
			comp.Verbosity = ui.Hidden
		}
	case model.InfraFailure, model.Failure:
		comp.Verbosity = ui.Interesting
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
		comp.MainLink = ui.LinkSet{ub.BuildLink(anno.Link)}

		// If we also have a STDOUT stream, add it to our OtherLinks.
		if stdoutLink != nil {
			anno.OtherLinks = append([]*miloProto.Link{stdoutLink}, anno.OtherLinks...)
		}
	} else if stdoutLink != nil {
		comp.MainLink = ui.LinkSet{ub.BuildLink(stdoutLink)}
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
			comp.SubLink = append(comp.SubLink, ui.LinkSet{l})
		}
	}

	// This should always be a step.
	comp.Type = ui.Step

	// This should always be 0
	comp.LevelsDeep = 0

	// Timestamps
	var start, end time.Time
	if t, err := ptypes.Timestamp(anno.Started); err == nil {
		start = t
	}
	if t, err := ptypes.Timestamp(anno.Ended); err == nil {
		end = t
	}
	comp.ExecutionTime = ui.NewInterval(c, start, end)

	// This should be the exact same thing.
	comp.Text = append(comp.Text, anno.Text...)

	ss := anno.GetSubstep()
	results := []*ui.BuildComponent{}
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
		for _, subcomp := range miloBuildStep(c, ub, subanno) {
			results = append(results, subcomp)
		}
	}

	return results
}

// SubStepsToUI converts a slice of annotation substeps to ui.BuildComponent and
// slice of ui.PropertyGroups.
func SubStepsToUI(c context.Context, ub URLBuilder, substeps []*miloProto.Step_Substep) ([]*ui.BuildComponent, []*ui.PropertyGroup) {
	components := make([]*ui.BuildComponent, 0, len(substeps))
	propGroups := make([]*ui.PropertyGroup, 0, len(substeps)+1) // This is the max number or property groups.
	for _, substepContainer := range substeps {
		anno := substepContainer.GetStep()
		if anno == nil {
			// TODO: We ignore non-embedded substeps for now.
			continue
		}

		bss := miloBuildStep(c, ub, anno)
		for _, bs := range bss {
			components = append(components, bs)
			propGroup := &ui.PropertyGroup{GroupName: bs.Label.Label}
			for _, prop := range anno.Property {
				propGroup.Property = append(propGroup.Property, &ui.Property{
					Key:   prop.Name,
					Value: prop.Value,
				})
			}
			propGroups = append(propGroups, propGroup)
		}
	}

	return components, propGroups
}

// AddLogDogToBuild takes a set of logdog streams and populate a milo build.
// build.Summary.Finished must be set.
func AddLogDogToBuild(
	c context.Context, ub URLBuilder, mainAnno *miloProto.Step, build *ui.MiloBuild) {

	// Now fill in each of the step components.
	// TODO(hinoka): This is totes cachable.
	build.Summary = *(miloBuildStep(c, ub, mainAnno)[0])
	build.Components, build.PropertyGroup = SubStepsToUI(c, ub, mainAnno.Substep)

	// Take care of properties
	propGroup := &ui.PropertyGroup{GroupName: "Main"}
	for _, prop := range mainAnno.Property {
		propGroup.Property = append(propGroup.Property, &ui.Property{
			Key:   prop.Name,
			Value: prop.Value,
		})
	}
	build.PropertyGroup = append(build.PropertyGroup, propGroup)

	// Build a property map so we can extract revision properties.
	propMap := map[string]string{}
	for _, pg := range build.PropertyGroup {
		for _, p := range pg.Property {
			propMap[p.Key] = p.Value
		}
	}
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
				build.Trigger = &ui.Trigger{}
			}
			build.Trigger.Revision = ui.NewLink(
				rev, fmt.Sprintf("https://crrev.com/%s", rev), fmt.Sprintf("revision %s", rev))
		}
	}

	return
}
