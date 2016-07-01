// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logdog

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	miloProto "github.com/luci/luci-go/common/proto/milo"
)

// Given a logdog/milo step, translate it to a BuildComponent struct.
func miloBuildStep(c context.Context, url string, anno *miloProto.Step) *resp.BuildComponent {
	url = strings.TrimSuffix(url, "/")
	comp := &resp.BuildComponent{Label: anno.Name}
	switch anno.Status {
	case miloProto.Status_RUNNING:
		comp.Status = resp.Running

	case miloProto.Status_SUCCESS:
		comp.Status = resp.Success

	case miloProto.Status_FAILURE:
		if anno.GetFailureDetails() != nil {
			switch anno.GetFailureDetails().Type {
			case miloProto.FailureDetails_EXCEPTION, miloProto.FailureDetails_INFRA:
				comp.Status = resp.InfraFailure

			case miloProto.FailureDetails_DM_DEPENDENCY_FAILED:
				comp.Status = resp.DependencyFailure

			default:
				comp.Status = resp.Failure
			}
		} else {
			comp.Status = resp.Failure
		}

		// Missing the case of waiting on unfinished dependency...
	default:
		comp.Status = resp.NotRun
	}
	// Sub link is for one link per log that isn't stdout.
	for _, link := range anno.GetOtherLinks() {
		lds := link.GetLogdogStream()
		if lds == nil {
			logging.Warningf(c, "Warning: %v of %v has an empty logdog stream.", link, anno)
			continue // DNE???
		}
		newLink := &resp.Link{
			Label: lds.Name,
			URL:   url + "/" + lds.Name,
		}
		comp.SubLink = append(comp.SubLink, newLink)
	}

	// Main link is a link to the stdout.
	if anno.StdoutStream != nil {
		comp.MainLink = &resp.Link{
			Label: "stdout",
			URL:   makeLogDogStreamURL(url, anno.StdoutStream),
		}
	}

	// This should always be a step.
	comp.Type = resp.Step

	// This should always be 0
	comp.LevelsDeep = 0

	// Timestamps
	comp.Started = anno.Started.Time()
	comp.Finished = anno.Ended.Time()

	till := comp.Finished
	if anno.Status == miloProto.Status_RUNNING {
		till = clock.Now(c)
	}
	if !comp.Started.IsZero() && !till.IsZero() {
		comp.Duration = till.Sub(comp.Started)
	}

	// This should be the exact same thing.
	comp.Text = anno.Text

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
	for _, substepContainer := range mainAnno.Substep {
		anno := substepContainer.GetStep()
		if anno == nil {
			// TODO: We ignore non-embedded substeps for now.
			continue
		}

		bs := miloBuildStep(c, url, anno)
		if bs.Status != resp.Success && bs.Status != resp.NotRun {
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
		}
		build.PropertyGroup = append(build.PropertyGroup, propGroup)
	}

	// Take care of properties
	propGroup := &resp.PropertyGroup{GroupName: "Main"}
	for _, prop := range mainAnno.Property {
		propGroup.Property = append(propGroup.Property, &resp.Property{
			Key:   prop.Name,
			Value: prop.Value,
		})
	}
	build.PropertyGroup = append(build.PropertyGroup, propGroup)

	return
}

func makeLogDogStreamURL(urlBase string, s *miloProto.LogdogStream) string {
	return urlBase + "/" + s.Name
}
