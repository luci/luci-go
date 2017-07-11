// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"strconv"
	"strings"
	"unicode/utf8"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	milo "github.com/luci/luci-go/milo/api/proto"
	"github.com/luci/luci-go/swarming/tasktemplate"

	"golang.org/x/net/context"
)

// BuildInfoProvider provides build information.
//
// In a production system, this will be completely defaults. For testing, the
// various services and data sources may be substituted for testing stubs.
type BuildInfoProvider struct {
	bl BuildLoader

	// swarmingServiceFunc returns a swarmingService instance for the supplied
	// parameters.
	//
	// If nil, a production fetcher will be generated.
	swarmingServiceFunc func(c context.Context, host string) (swarmingService, error)
}

func (p *BuildInfoProvider) newSwarmingService(c context.Context, host string) (swarmingService, error) {
	if p.swarmingServiceFunc == nil {
		return NewProdService(c, host)
	}
	return p.swarmingServiceFunc(c, host)
}

// GetBuildInfo resolves a Milo protobuf Step for a given Swarming task.
func (p *BuildInfoProvider) GetBuildInfo(c context.Context, req *milo.BuildInfoRequest_Swarming,
	projectHint cfgtypes.ProjectName) (*milo.BuildInfoResponse, error) {

	// Load the Swarming task (no log content).
	sf, err := p.newSwarmingService(c, req.Host)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to create Swarming fetcher.")
		return nil, grpcutil.Internal
	}

	// Use default Swarming host.
	host := sf.getHost()
	logging.Infof(c, "Loading build info for Swarming host %s, task %s.", host, req.Task)

	fetchParams := swarmingFetchParams{
		fetchReq: true,
		fetchRes: true,
	}
	fr, err := swarmingFetch(c, sf, req.Task, fetchParams)
	if err != nil {
		if err == errNotMiloJob {
			logging.Warningf(c, "User requested nonexistent task or does not have permissions.")
			return nil, grpcutil.NotFound
		}

		logging.WithError(err).Errorf(c, "Failed to load Swarming task.")
		return nil, grpcutil.Internal
	}

	// Determine the LogDog annotation stream path for this Swarming task.
	//
	// On failure, will return a gRPC error.
	stream, err := resolveLogDogAnnotations(c, fr.req, projectHint, host, req.Task, fr.res.TryNumber)
	if err != nil {
		logging.WithError(err).Warningf(c, "Failed to get annotation stream parameters.")
		return nil, err
	}

	logging.Fields{
		"host":    stream.Host,
		"project": stream.Project,
		"path":    stream.Path,
	}.Infof(c, "Resolved LogDog annotation stream.")

	as, err := p.bl.newEmptyAnnotationStream(c, stream)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to create LogDog annotation stream.")
		return nil, grpcutil.Internal
	}

	// Fetch LogDog annotation stream data.
	step, err := as.Fetch(c)
	if err != nil {
		logging.WithError(err).Warningf(c, "Failed to load annotation stream.")
		return nil, grpcutil.Internal
	}

	// Add Swarming task parameters to the Milo step.
	if err := addTaskToMiloStep(c, sf.getHost(), fr.res, step); err != nil {
		return nil, err
	}

	prefix, name := as.Path.Split()
	return &milo.BuildInfoResponse{
		Project: string(as.Project),
		Step:    step,
		AnnotationStream: &miloProto.LogdogStream{
			Server: as.Client.Host,
			Prefix: string(prefix),
			Name:   string(name),
		},
	}, nil
}

// resolveLogDogAnnotations returns a configured AnnotationStream given the input
// parameters.
//
// This will return a gRPC error on failure.
//
// This function is messy and implementation-specific. That's the point of this
// endpoint, though. All of the nastiness here should be replaced with something
// more elegant once that becomes available. In the meantime...
func resolveLogDogAnnotations(c context.Context, sr *swarming.SwarmingRpcsTaskRequest, projectHint cfgtypes.ProjectName,
	host, taskID string, tryNumber int64) (*types.StreamAddr, error) {

	// Try and resolve from explicit tags (preferred).
	tags := swarmingTags(sr.Tags)
	addr, err := resolveLogDogStreamAddrFromTags(tags, taskID, tryNumber)
	if err == nil {
		return addr, nil
	}
	logging.WithError(err).Debugf(c, "Could not resolve stream address from tags.")

	// If this is a Kitchen command, maybe we can infer our LogDog project from
	// the command-line.
	if sr.Properties == nil {
		logging.Warningf(c, "No request properties, can't infer annotation stream path.")
		return nil, grpcutil.NotFound
	}

	addr = &types.StreamAddr{}
	var isKitchen bool
	if addr.Project, isKitchen = getLogDogProjectFromKitchen(sr.Properties.Command); !isKitchen {
		logging.Warningf(c, "Not a Kitchen CLI, can't infer annotation stream path.")
		return nil, grpcutil.NotFound
	}

	if addr.Project == "" {
		addr.Project = projectHint
	}
	if addr.Project == "" {
		logging.Warningf(c, "Don't know how to get annotation stream path.")
		return nil, grpcutil.NotFound
	}

	// This is a Kitchen run, and it has a project! Construct the annotation
	// stream path.
	//
	// This is an implementation of:
	// https://chromium.googlesource.com/infra/infra/+/a7032e3e240d4b81a1912bfaf29a20d02f665cc1/go/src/infra/tools/kitchen/cook_logdog.go#129
	runID, err := getRunID(taskID, tryNumber)
	if err != nil {
		logging.Fields{
			logging.ErrorKey: err,
			"taskID":         taskID,
			"tryNumber":      tryNumber,
		}.Errorf(c, "Failed to get Run ID for task/try.")
		return nil, grpcutil.Internal
	}

	prefix, err := types.MakeStreamName("", "swarm", host, runID)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to generate Swarming prefix.")
		return nil, grpcutil.Internal
	}
	addr.Path = prefix.Join("annotations")
	return addr, nil
}

func getLogDogProjectFromKitchen(cmd []string) (proj cfgtypes.ProjectName, isKitchen bool) {
	// Is this a Kitchen command?
	switch {
	case len(cmd) == 0:
		return
	case !strings.HasPrefix(cmd[0], "kitchen"):
		return
	}
	isKitchen = true
	cmd = cmd[1:]

	// Scan through for the "-logdog-project" argument.
	for i, arg := range cmd {
		if arg == "-logdog-project" {
			if i < len(cmd)-2 {
				proj = cfgtypes.ProjectName(cmd[i+1])
				return
			}
			break
		}
	}
	return
}

func resolveLogDogStreamAddrFromTags(tags map[string]string, taskID string, tryNumber int64) (*types.StreamAddr, error) {
	// If we don't have a LUCI project, abort.
	luciProject, logLocation := tags["luci_project"], tags["log_location"]
	switch {
	case luciProject == "":
		return nil, errors.New("no 'luci_project' tag")
	case logLocation == "":
		return nil, errors.New("no 'log_location' tag")
	}

	// Gather our Swarming task template parameters and perform a substitution.
	runID, err := getRunID(taskID, tryNumber)
	if err != nil {
		return nil, errors.Annotate(err, "").Err()
	}
	p := tasktemplate.Params{
		SwarmingRunID: runID,
	}
	if logLocation, err = p.Resolve(logLocation); err != nil {
		return nil, errors.Annotate(err, "failed to resolve swarming task templating in 'log_location'").Err()
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

// getRunID converts a Swarming task ID and try number into a Swarming Run ID.
//
// The run ID is a permutation of the last four bits of the Swarming Task ID.
// Therefore, we chop it off of the string, mutate it, and then add it back.
//
// TODO(dnj): Replace this with Swarming API call once finished.
func getRunID(taskID string, tryNumber int64) (string, error) {
	// Slice off the last character form the task ID.
	if len(taskID) == 0 {
		return "", errors.New("swarming task ID is empty")
	}

	// Parse "tryNumber" as a hex string.
	if tryNumber < 0 || tryNumber > 0x0F {
		return "", errors.Reason("try number %d exceeds 4 bits", tryNumber).Err()
	}

	lastChar, lastCharSize := utf8.DecodeLastRuneInString(taskID)
	v, err := strconv.ParseUint(string(lastChar), 16, 8)
	if err != nil {
		return "", errors.Annotate(err, "failed to parse hex from rune: %r", lastChar).Err()
	}

	return taskID[:len(taskID)-lastCharSize] + strconv.FormatUint((v|uint64(tryNumber)), 16), nil
}
