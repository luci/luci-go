// Copyright 2017 The LUCI Authors.
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

package buildstore

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.chromium.org/luci/buildbucket"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
)

// This file implements conversion of buildbucket builds to buildbot builds.

// ErrNoBuildNumber is an error tag indicating that a build does not have
// a build number.
var ErrNoBuildNumber = errors.BoolTag{Key: errors.NewTagKey("no buildnumber")}

// buildFromBuildbucket converts a buildbucket build to a buildbot build.
// If details is false, steps, text and properties are not guaranteed to be
// loaded.
//
// Does not populate OSFamily, OSVersion, Blame or SourceStamp.Changes
// fields.
func buildFromBuildbucket(c context.Context, master string, b *buildbucket.Build, fetchAnnotations bool) (*buildbot.Build, error) {
	num, err := buildNumber(b)
	if err != nil {
		return nil, err
	}

	res := &buildbot.Build{
		Emulated:    true,
		ViewPath:    (&model.BuildSummary{BuildID: "buildbucket/" + b.Address()}).SelfLink(),
		Master:      master,
		Buildername: b.Builder,
		Number:      num,
		Results:     statusResult(b.Status),
		TimeStamp:   buildbot.Time{b.UpdateTime},
		Times: buildbot.TimeRange{
			Start:  buildbot.Time{b.StartTime},
			Finish: buildbot.Time{b.CompletionTime},
		},
		// TODO(nodir): use buildbucket access API when it is ready,
		// or, perhaps, just delete this code.
		// Note that this function should not be called on builds
		// that the requester does not have access to, in the first place.
		Internal:    b.Bucket != "luci.chromium.try",
		Finished:    b.Status.Ended(),
		Sourcestamp: &buildbot.SourceStamp{},

		// non-nil slice because we want an array, not null,
		// when converted to JSON
		Steps: []buildbot.Step{},
	}

	for _, bs := range b.BuildSets {
		if commit, ok := bs.(*buildbucketpb.GitilesCommit); ok {
			res.Sourcestamp.Repository = commit.RepoURL()
			res.Sourcestamp.Revision = commit.Id
			break
		}
	}

	if fetchAnnotations && b.Status != buildbucketpb.Status_SCHEDULED {
		addr, err := logLocation(b)
		if err != nil {
			return nil, err
		}

		ann, err := fetchAnnotationProto(c, addr)
		switch {
		case err == errAnnotationNotFound:
		case err != nil:
			return nil, errors.Annotate(err, "could not load annotation proto").Err()
		default:
			res.Properties = extractProperties(ann)

			prefix, _ := addr.Path.Split()
			conv := annotationConverter{
				logdogServer:       addr.Host,
				logdogPrefix:       fmt.Sprintf("%s/%s", addr.Project, prefix),
				buildCompletedTime: b.CompletionTime,
			}
			convCtx := logging.SetField(c, "build_id", b.ID)
			conv.addSteps(convCtx, &res.Steps, ann.Substep, "")
			for i := range res.Steps {
				s := &res.Steps[i]
				s.StepNumber = i + 1
				res.Logs = append(res.Logs, s.Logs...)
				if !s.IsFinished {
					res.Currentstep = s.Name
				}
				if s.Results.Result != buildbot.Success {
					res.Text = append(res.Text, fmt.Sprintf("%s %s", s.Results.Result, s.Name))
				}
			}
		}
	}

	if len(res.Text) == 0 && b.Status == buildbucketpb.Status_SUCCESS {
		res.Text = []string{"Build successful"}
	}

	return res, nil
}

func buildbucketClient(c context.Context) (*bbv1.Service, error) {
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, err
	}
	client, err := bbv1.New(&http.Client{Transport: t})
	if err != nil {
		return nil, err
	}

	settings := common.GetSettings(c)
	if settings.GetBuildbucket().GetHost() == "" {
		return nil, errors.New("missing buildbucket host in settings")
	}

	client.BasePath = fmt.Sprintf("https://%s/_ah/api/buildbucket/v1/", settings.Buildbucket.Host)
	return client, nil
}

func buildNumber(b *buildbucket.Build) (int, error) {
	address := b.Tags.Get("build_address")
	if address == "" {
		return 0, errors.Reason("no build_address in build %d", b.ID).Tag(ErrNoBuildNumber).Err()
	}

	// address format is "<bucket>/<builder>/<buildnumber>"
	parts := strings.Split(address, "/")
	if len(parts) != 3 {
		return 0, errors.Reason("unexpected build_address format, %q", address).Err()
	}

	return strconv.Atoi(parts[2])
}

func statusResult(status buildbucketpb.Status) buildbot.Result {
	switch status {
	case buildbucketpb.Status_SCHEDULED, buildbucketpb.Status_STARTED:
		return buildbot.NoResult
	case buildbucketpb.Status_SUCCESS:
		return buildbot.Success
	case buildbucketpb.Status_FAILURE:
		return buildbot.Failure
	case buildbucketpb.Status_INFRA_FAILURE, buildbucketpb.Status_CANCELED:
		return buildbot.Exception
	default:
		panic(errors.Reason("unexpected buildbucket status %q", status).Err())
	}
}

func logLocation(b *buildbucket.Build) (*types.StreamAddr, error) {
	swarmingTags := strpair.ParseMap(b.Tags["swarming_tag"])
	logLocation := swarmingTags.Get("log_location")
	if logLocation == "" {
		return nil, errors.New("log_location not found")
	}

	// Parse LogDog URL
	addr, err := types.ParseURL(logLocation)
	if err != nil {
		return nil, errors.Annotate(err, "could not parse LogDog stream from location").Err()
	}
	return addr, nil
}
