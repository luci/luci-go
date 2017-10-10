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
	"io"
	"net/http"
	"strconv"
	"strings"


	"golang.org/x/net/context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/api/buildbot"
)

// This file implements conversion of buildbucket builds to buildbot builds.

// buildFromBuildbucket converts a buildbucket build to a buildbot build.
//
// Does not populate Blame or SourceStamp.
func buildFromBuildbucket(c context.Context, msg *bbapi.ApiCommonBuildMessage, numbersOnly bool) (*buildbot.Build, error) {
	var b buildbucket.Build
	if err := b.ParseMessage(msg); err != nil {
		return nil, err
	}
	num, err := buildNumber(&b)
	if err != nil {
		return nil, errors.Annotate(err, "parsing buildnumber").Err()
	}
	res := &buildbot.Build{
		Master:      b.Tags.Get("mastername"),
		Buildername: b.Builder,
		Number:      num,
		Results:     statusResult(b.Status),
		Times: buildbot.TimeRange{
			Start:  buildbot.Time{b.StartTime},
			Finish: buildbot.Time{b.CompletionTime},
		},
		// TODO(nodir): use buildbucket access API when it is ready,
		// or delete simply delete this code.
		Internal: b.Bucket == "luci.chromium.try",
		Finished: b.Status.Completed(),
	}
	if numbersOnly {
		return res, nil
	}

	properties := b.Input.UserData
	if b.Output.UserData != nil {
		properties = b.Output.UserData
	}

	for k, v := range properties.(map[string]interface{}) {
		res.Properties = append(res.Properties, &buildbot.Property{
			Name:   k,
			Value:  v,
			Source: "buildbucket",
		})
	}

	if b.Status != buildbucket.StatusScheduled {
		// Use annotation proto only for steps.
		addr, err := logLocation(&b)
		if err != nil {
			return nil, err
		}

		ann, err := fetchAnnotationProto(c, addr)
		if err != io.EOF {
			if err != nil {
				return nil, errors.Annotate(err, "could not load annotation proto").Err()
			}

			prefix, _ := addr.Path.Split()
			conv := annotationConverter{
				logdogServer:       addr.Host,
				logdogPrefix:       string(prefix),
				buildCompletedTime: b.CompletionTime,
			}
			convCtx := logging.SetField(c, "build_id", b.ID)
			conv.addSteps(convCtx, res, ann.Substep, "")
			for i := range res.Steps {
				res.Steps[i].StepNumber = i + 1
			}
		}
	}

	return res, nil
}

func buildbucketClient(c context.Context) (*bbapi.Service, error) {
	// make it anonymous to avoid leaking internal information accidentally
	transport, err := auth.GetRPCTransport(c, auth.NoAuth)
	if err != nil {
		return nil, errors.Annotate(err, "could not get RPC transport").Err()
	}

	service, err := bbapi.New(&http.Client{Transport: transport})
	if err != nil {
		return nil, err
	}
	service.BasePath = "https://12088-7f9b1c2-tainted-nodir-dot-cr-buildbucket-dev.appspot.com/api/buildbucket/v1/"
	return service, nil

	return bbapi.New(&http.Client{Transport: transport})
}

func buildNumber(b *buildbucket.Build) (int, error) {
	address := b.Tags.Get("build_address")
	if address == "" {
		return 0, errors.Reason("no build_address", b.ID).Err()
	}

	// address format is "<bucket>/<builder>/<buildnumber>"
	parts := strings.Split(address, "/")
	if len(parts) != 3 {
		return 0, errors.Reason("unexpected build_address format, %q", address).Err()
	}

	return strconv.Atoi(parts[2])
}

func statusResult(status buildbucket.Status) buildbot.Result {
	switch status {
	case buildbucket.StatusScheduled, buildbucket.StatusStarted:
		return buildbot.NoResult
	case buildbucket.StatusSuccess:
		return buildbot.Success
	case buildbucket.StatusFailure:
		return buildbot.Failure
	case buildbucket.StatusError, buildbucket.StatusTimeout, buildbucket.StatusCancelled:
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
