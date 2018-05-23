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

package notify

import (
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/common/viewer"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/frontend/ui"
)

type Step struct {
	Status model.Status
	Name string
	Text string
	Links []ui.Link
}

type Annotations struct {
	FailedSteps []Step
}

func getBuildAnnotations(c context.Context, b *Build) (*Annotations, error) {
	rawSwarmingTags, ok := b.Tags["swarming_tag"]
	if !ok {
		return nil, nil
	}
	swarmingTags := strpair.ParseMap(rawSwarmingTags)
	logLocation, ok := swarmingTags["log_location"]
	if !ok {
		return nil, nil
	}
	addr, err := types.ParseURL(logLocation)
	if err != nil {
		return nil, errors.Annotate(err, "%s is invalid", addr).Err()
	}
	step, err := rawpresentation.ReadAnnotations(c, addr)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read annotations").Err()
	}
	prefix, _ := addr.Path.Split()
	urlBuilder := ViewerURLBuilder{
		Host:    addr.Host,
		Prefix:  prefix,
		Project: addr.Project,
	}
	annotations := Annotations{}
	processAnnotations(&urlBuilder, annotations, step)
	return &annotations, nil
}

func processSteps(ub *ViewerURLBuilder, a *Annotations, step *milo.Step) {
	if step.Status == milo.Status_FAILURE:
		text := step.Text
		var status model.Status
		if fd := anno.GetFailureDetails(); fd != nil {
			switch fd.Type {
			case milo.FailureDetails_EXCEPTION, milo.FailureDetails_INFRA:
				status = model.InfraFailure
			case milo.FailureDetails_EXPIRED:
				status = model.Expired
			default:
				status = model.Failure
			}
			if fd.Text != "" {
				text = append(text, fd.Text)
			}
		} else {
			status = model.Failure
		}
		var links []Link
		if anno.StdoutStream != nil {
			step.OtherLinks = append(step.OtherLinks, &milo.Link{
				Label: "stdout",
				Value: &milo.Link_LogdogStream{
					LogdogStream: anno.StdoutStream,
				},
			})
		}
		if anno.StderrStream != nil {
			step.OtherLinks = append(step.OtherLinks, &milo.Link{
				Label: "stderr",
				Value: &milo.Link_LogdogStream{
					LogdogStream: anno.StderrStream,
				},
			})
		}
		for _, l := range step.OtherLinks {
			links = append(links, ub.BuildLink(l))
		}
		a.FailedSteps = append(a.FailedSteps, Step{
			Status: status,
			Name: step.Name,
			Text: strings.Join(text, "\n"),
			Links: links,
		})
	}
	for _, step := range step.Substep {
		next := step.GetStep()
		if next != nil {
			processSteps(ub, a, next)
		}
	}
}
