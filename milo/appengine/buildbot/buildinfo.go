// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/logging"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	milo "github.com/luci/luci-go/milo/api/proto"
	"github.com/luci/luci-go/milo/appengine/logdog"

	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"
)

// BuildInfoProvider is a configuration that provides build information.
//
// In a production system, this will be completely defaults. For testing, the
// various services and data sources may be substituted for testing stubs.
type BuildInfoProvider struct {
	// LogdogClientFunc returns a coordinator Client instance for the supplied
	// parameters.
	//
	// If nil, a production client will be generated.
	LogdogClientFunc func(c context.Context) (*coordinator.Client, error)
}

func (p *BuildInfoProvider) newLogdogClient(c context.Context) (*coordinator.Client, error) {
	if p.LogdogClientFunc != nil {
		return p.LogdogClientFunc(c)
	}
	return logdog.NewClient(c, "")
}

// GetBuildInfo resolves a Milo protobuf Step for a given BuildBot build.
//
// On failure, it returns a (potentially-wrapped) gRPC error.
//
// This:
//
//	1) Fetches the BuildBot build JSON from datastore.
//	2) Resolves the LogDog annotation stream path from the BuildBot state.
//	3) Fetches the LogDog annotation stream and resolves it into a Step.
//	4) Merges some operational BuildBot build information into the Step.
func (p *BuildInfoProvider) GetBuildInfo(c context.Context, req *milo.BuildInfoRequest_BuildBot,
	projectHint cfgtypes.ProjectName) (*milo.BuildInfoResponse, error) {

	logging.Infof(c, "Loading build info for master %q, builder %q, build #%d",
		req.MasterName, req.BuilderName, req.BuildNumber)

	// Load the BuildBot build from datastore.
	build, err := getBuild(c, req.MasterName, req.BuilderName, int(req.BuildNumber))
	if err != nil {
		if err == errBuildNotFound {
			return nil, grpcutil.Errf(codes.NotFound, "Build #%d for master %q, builder %q was not found",
				req.BuildNumber, req.MasterName, req.BuilderName)
		}

		logging.WithError(err).Errorf(c, "Failed to load build info.")
		return nil, grpcutil.Internal
	}

	// Create a new LogDog client.
	client, err := p.newLogdogClient(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to create LogDog client.")
		return nil, grpcutil.Internal
	}

	// Identify the LogDog annotation stream from the build.
	//
	// This will return a gRPC error on failure.
	addr, err := getLogDogAnnotationAddr(c, client, build, projectHint)
	if err != nil {
		return nil, err
	}
	logging.Infof(c, "Resolved annotation stream: %s / %s", addr.Project, addr.Path)

	// Load the annotation protobuf.
	as := logdog.AnnotationStream{
		Client:  client,
		Path:    addr.Path,
		Project: addr.Project,
	}
	if err := as.Normalize(); err != nil {
		logging.WithError(err).Errorf(c, "Failed to normalize annotation stream.")
		return nil, grpcutil.Internal
	}

	step, err := as.Fetch(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to load annotation stream.")
		return nil, grpcutil.Errf(codes.Internal, "failed to load LogDog annotation stream from: %s", as.Path)
	}

	// Merge the information together.
	if err := mergeBuildIntoAnnotation(c, step, build); err != nil {
		logging.WithError(err).Errorf(c, "Failed to merge annotation with build.")
		return nil, grpcutil.Errf(codes.Internal, "failed to merge annotation and build data")
	}

	prefix, name := as.Path.Split()
	return &milo.BuildInfoResponse{
		Project: string(as.Project),
		Step:    step,
		AnnotationStream: &miloProto.LogdogStream{
			Server: client.Host,
			Prefix: string(prefix),
			Name:   string(name),
		},
	}, nil
}

// Resolve BuildBot and LogDog build information. We do this
//
// This returns an AnnotationStream instance with its project and path
// populated.
//
// This function is messy and implementation-specific. That's the point of this
// endpoint, though. All of the nastiness here should be replaced with something
// more elegant once that becomes available. In the meantime...
func getLogDogAnnotationAddr(c context.Context, client *coordinator.Client, build *buildbotBuild,
	projectHint cfgtypes.ProjectName) (*types.StreamAddr, error) {

	if v, ok := build.getPropertyValue("log_location").(string); ok && v != "" {
		addr, err := types.ParseURL(v)
		if err == nil {
			return addr, nil
		}

		logging.Fields{
			logging.ErrorKey: err,
			"log_location":   v,
		}.Debugf(c, "'log_location' property did not parse as LogDog URL.")
	}

	// logdog_annotation_url (if present, must be valid)
	if v, ok := build.getPropertyValue("logdog_annotation_url").(string); ok && v != "" {
		addr, err := types.ParseURL(v)
		if err != nil {
			logging.Fields{
				logging.ErrorKey: err,
				"url":            v,
			}.Errorf(c, "Failed to parse 'logdog_annotation_url' property.")
			return nil, grpcutil.Errf(codes.FailedPrecondition, "build has invalid annotation URL")
		}

		return addr, nil
	}

	// Modern builds will have this information in their build properties.
	var addr types.StreamAddr
	prefix, _ := build.getPropertyValue("logdog_prefix").(string)
	project, _ := build.getPropertyValue("logdog_project").(string)
	if prefix != "" && project != "" {
		// Construct the full annotation path.
		addr.Project = cfgtypes.ProjectName(project)
		addr.Path = types.StreamName(prefix).Join("annotations")

		logging.Debugf(c, "Resolved path/project from build properties.")
		return &addr, nil
	}

	// From here on out, we will need a project hint.
	if projectHint == "" {
		return nil, grpcutil.Errf(codes.NotFound, "annotation stream not found")
	}
	addr.Project = projectHint

	// Execute a LogDog service query to see if we can identify the stream.
	err := func() error {
		var annotationStream *coordinator.LogStream
		err := client.Query(c, addr.Project, "", coordinator.QueryOptions{
			Tags: map[string]string{
				"buildbot.master":      build.Master,
				"buildbot.builder":     build.Buildername,
				"buildbot.buildnumber": strconv.Itoa(build.Number),
			},
			ContentType: miloProto.ContentTypeAnnotations,
		}, func(ls *coordinator.LogStream) bool {
			// Only need the first (hopefully only?) result.
			annotationStream = ls
			return false
		})
		if err != nil {
			logging.WithError(err).Errorf(c, "Failed to issue log stream query.")
			return grpcutil.Internal
		}

		if annotationStream != nil {
			addr.Path = annotationStream.Path
		}
		return nil
	}()
	if err != nil {
		return nil, err
	}
	if addr.Path != "" {
		logging.Debugf(c, "Resolved path/project via tag query.")
		return &addr, nil
	}

	// Last-ditch effort: generate a prefix based on the build properties. This
	// re-implements the "_build_prefix" function in:
	// https://chromium.googlesource.com/chromium/tools/build/+/2d23e5284cc31f31c6bc07aa1d3fc5b1c454c3b4/scripts/slave/logdog_bootstrap.py#363
	isAlnum := func(r rune) bool {
		return ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}
	normalize := func(v string) string {
		v = strings.Map(func(r rune) rune {
			if isAlnum(r) {
				return r
			}
			switch r {
			case ':', '_', '-', '.':
				return r
			default:
				return '_'
			}
		}, v)
		if r, _ := utf8.DecodeRuneInString(v); r == utf8.RuneError || !isAlnum(r) {
			v = "s_" + v
		}
		return v
	}
	addr.Path = types.StreamPath(fmt.Sprintf("bb/%s/%s/%s/+/annotations",
		normalize(build.Master), normalize(build.Buildername), normalize(strconv.Itoa(build.Number))))

	logging.Debugf(c, "Generated path/project algorithmically.")
	return &addr, nil
}

// mergeBuildInfoIntoAnnotation merges BuildBot-specific build informtion into
// a LogDog annotation protobuf.
//
// This consists of augmenting the Step's properties with BuildBot's properties,
// favoring the Step's version of the properties if there are two with the same
// name.
func mergeBuildIntoAnnotation(c context.Context, step *miloProto.Step, build *buildbotBuild) error {
	allProps := stringset.New(len(step.Property) + len(build.Properties))
	for _, prop := range step.Property {
		allProps.Add(prop.Name)
	}
	for _, prop := range build.Properties {
		// Annotation protobuf overrides BuildBot properties.
		if allProps.Has(prop.Name) {
			continue
		}
		allProps.Add(prop.Name)

		step.Property = append(step.Property, &miloProto.Step_Property{
			Name:  prop.Name,
			Value: fmt.Sprintf("%v", prop.Value),
		})
	}

	return nil
}
