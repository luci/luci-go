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

package swarming

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/common/types"
)

// resolveLogDogAnnotations returns a configured AnnotationStream given the input
// parameters.
//
// This will return a gRPC error on failure.
func resolveLogDogAnnotations(c context.Context, tagsRaw []string) (*types.StreamAddr, error) {
	addr, err := resolveLogDogStreamAddrFromTags(swarmingTags(tagsRaw))
	if err != nil {
		logging.WithError(err).Debugf(c, "Could not resolve stream address from tags.")
		return nil, grpcutil.NotFound
	}
	return addr, nil
}

func resolveLogDogStreamAddrFromTags(tags map[string]string) (*types.StreamAddr, error) {
	// If we don't have a LUCI project, abort.
	luciProject, logLocation := tags["luci_project"], tags["log_location"]
	switch {
	case luciProject == "":
		return nil, errors.New("no 'luci_project' tag")
	case logLocation == "":
		return nil, errors.New("no 'log_location' tag")
	}

	addr, err := types.ParseURL(logLocation)
	if err != nil {
		return nil, errors.Annotate(err, "could not parse LogDog stream from location").Err()
	}

	// The LogDog stream's project should match the LUCI project.
	if string(addr.Project) != luciProject {
		return nil, errors.Reason("stream project %q doesn't match LUCI project %q", addr.Project, luciProject).Err()
	}

	return addr, nil
}
