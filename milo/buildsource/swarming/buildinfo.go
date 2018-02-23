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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/common/types"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"

	"golang.org/x/net/context"
)

// BuildInfoProvider provides build information.
//
// In a production system, this will be completely defaults. For testing, the
// various services and data sources may be substituted for testing stubs.
type BuildInfoProvider struct {
	// swarmingServiceFunc returns a SwarmingService instance for the supplied
	// parameters.
	//
	// If nil, a production fetcher will be generated.
	swarmingServiceFunc func(c context.Context, host string) (SwarmingService, error)
}

func (p *BuildInfoProvider) newSwarmingService(c context.Context, host string) (SwarmingService, error) {
	if p.swarmingServiceFunc == nil {
		return NewProdService(c, host)
	}
	return p.swarmingServiceFunc(c, host)
}

// GetBuildInfo resolves a Milo protobuf Step for a given Swarming task.
func (p *BuildInfoProvider) GetBuildInfo(c context.Context, req *milo.BuildInfoRequest_Swarming,
	projectHint config.ProjectName) (*milo.BuildInfoResponse, error) {

	// Load the Swarming task (no log content).
	sf, err := p.newSwarmingService(c, req.Host)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to create Swarming fetcher.")
		return nil, grpcutil.Internal
	}

	// Use default Swarming host.
	host := sf.GetHost()
	logging.Infof(c, "Loading build info for Swarming host %s, task %s.", host, req.Task)

	fr, err := swarmingFetch(c, sf, req.Task, swarmingFetchParams{})
	if err != nil {
		if err == ErrNotMiloJob {
			logging.Warningf(c, "User requested nonexistent task or does not have permissions.")
			return nil, grpcutil.NotFound
		}

		logging.WithError(err).Errorf(c, "Failed to load Swarming task.")
		return nil, grpcutil.Internal
	}

	// Determine the LogDog annotation stream path for this Swarming task.
	//
	// On failure, will return a gRPC error.
	stream, err := resolveLogDogAnnotations(c, fr.res.Tags)
	if err != nil {
		logging.WithError(err).Warningf(c, "Failed to get annotation stream parameters.")
		return nil, err
	}

	logging.Fields{
		"host":    stream.Host,
		"project": stream.Project,
		"path":    stream.Path,
	}.Infof(c, "Resolved LogDog annotation stream.")

	step, err := rawpresentation.ReadAnnotations(c, stream)

	// Add Swarming task parameters to the Milo step.
	if err := addTaskToMiloStep(c, sf.GetHost(), fr.res, step); err != nil {
		return nil, err
	}

	prefix, name := stream.Path.Split()
	return &milo.BuildInfoResponse{
		Project: string(stream.Project),
		Step:    step,
		AnnotationStream: &miloProto.LogdogStream{
			Server: stream.Host,
			Prefix: string(prefix),
			Name:   string(name),
		},
	}, nil
}

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
