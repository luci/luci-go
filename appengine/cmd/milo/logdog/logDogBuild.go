// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logdog

import (
	"fmt"
	"path"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/logging"
	miloProto "github.com/luci/luci-go/common/proto/milo"
)

// Given a logdog/milo step, translate it to a BuildComponent struct.
func miloBuildStep(c context.Context, url string, anno *miloProto.Step, name string) *resp.BuildComponent {
	asc := anno.GetStepComponent()
	comp := &resp.BuildComponent{Label: asc.Name}
	switch asc.Status {
	case miloProto.Status_RUNNING:
		comp.Status = resp.Running

	case miloProto.Status_SUCCESS:
		comp.Status = resp.Success

	case miloProto.Status_FAILURE:
		if anno.GetFailureDetails() != nil {
			switch anno.GetFailureDetails().Type {
			case miloProto.FailureDetails_INFRA:
				comp.Status = resp.InfraFailure

			case miloProto.FailureDetails_DM_DEPENDENCY_FAILED:
				comp.Status = resp.DependencyFailure

			default:
				comp.Status = resp.Failure
			}
		} else {
			comp.Status = resp.Failure
		}

	case miloProto.Status_EXCEPTION:
		comp.Status = resp.InfraFailure

		// Missing the case of waiting on unfinished dependency...
	default:
		comp.Status = resp.NotRun
	}
	// Sub link is for one link per log that isn't stdout.
	for _, link := range asc.GetOtherLinks() {
		lds := link.GetLogdogStream()
		if lds == nil {
			logging.Warningf(c, "Warning: %v of %v has an empty logdog stream.", link, asc)
			continue // DNE???
		}
		segs := types.StreamName(lds.Name).Segments()
		switch segs[len(segs)-1] {
		case "stdout", "annotations":
			// Skip the special ones.
			continue
		}
		newLink := &resp.Link{
			Label: lds.Name,
			URL:   path.Join(url, lds.Name),
		}
		comp.SubLink = append(comp.SubLink, newLink)
	}

	// Main link is a link to the stdout.
	comp.MainLink = &resp.Link{
		Label: "stdout",
		URL:   path.Join(url, name, "stdout"),
	}

	// This should always be a step.
	comp.Type = resp.Step

	// This should always be 0
	comp.LevelsDeep = 0

	// Timestamps
	started := asc.Started.Time()
	ended := asc.Ended.Time()
	if !started.IsZero() {
		comp.Started = started.Format(time.RFC3339)
	}
	if !ended.IsZero() {
		comp.Finished = ended.Format(time.RFC3339)
	}

	till := ended
	if asc.Status == miloProto.Status_RUNNING {
		till = clock.Now(c)
	}
	if !started.IsZero() && !till.IsZero() {
		comp.Duration = uint64((till.Sub(started)) / time.Second)
	}

	// This should be the exact same thing.
	comp.Text = asc.Text

	return comp
}

// AddLogDogToBuild takes a set of logdog streams and populate a milo build.
func AddLogDogToBuild(c context.Context, url string, s *Streams, build *resp.MiloBuild) {
	if s.MainStream == nil {
		panic("missing main stream")
	}
	// Now Fetch the main annotation of the build.
	mainAnno := s.MainStream.Data

	// Now fill in each of the step components.
	// TODO(hinoka): This is totes cachable.
	for _, name := range mainAnno.SubstepLogdogNameBase {
		fullname := path.Join(name, "annotations")
		anno, ok := s.Streams[fullname]
		if !ok {
			// This should never happen, memory client already promised us that
			// theres an entry for every referenced stream.
			panic(fmt.Errorf("Could not find stream %s", fullname))
		}
		bs := miloBuildStep(c, url, anno.Data, name)
		build.Components = append(build.Components, bs)
		propGroup := &resp.PropertyGroup{GroupName: bs.Label}
		for _, prop := range anno.Data.GetStepComponent().Property {
			propGroup.Property = append(propGroup.Property, &resp.Property{
				Key:   prop.Name,
				Value: prop.Value,
			})
		}
		build.PropertyGroup = append(build.PropertyGroup, propGroup)
	}

	// Take care of properties
	propGroup := &resp.PropertyGroup{GroupName: "Main"}
	for _, prop := range mainAnno.GetStepComponent().Property {
		propGroup.Property = append(propGroup.Property, &resp.Property{
			Key:   prop.Name,
			Value: prop.Value,
		})
	}
	build.PropertyGroup = append(build.PropertyGroup, propGroup)

	return
}
