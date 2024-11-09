// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	log "go.chromium.org/luci/common/logging"
	cfglib "go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/server/config"
)

var projectConfigCtxKey = "logdog.coordinator.ProjectConfig"

// WithProjectNamespace sets the current namespace to the project name.
//
// Checks the project exists, but doesn't do any ACL checks.
//
// It will return a user-facing wrapped gRPC error on failure:
//   - InvalidArgument if the project name is invalid.
//   - PermissionDenied/Unauthenticated if the project doesn't exist.
//   - Internal if an internal error occurred.
func WithProjectNamespace(c *context.Context, project string) error {
	ctx := *c

	if err := cfglib.ValidateProjectName(project); err != nil {
		log.WithError(err).Errorf(ctx, "Project name is invalid.")
		return status.Errorf(codes.InvalidArgument, "Project name is invalid: %s", err)
	}

	// Load the project config, thus verifying the project exists.
	pcfg, err := config.ProjectConfig(ctx, project)
	switch {
	case err == cfglib.ErrNoConfig || err == config.ErrInvalidConfig:
		// If the configuration request was valid, but no configuration could be
		// loaded, treat this as the user not having READ access to the project.
		// Otherwise, the user could use this error response to confirm a
		// project's existence.
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
		}.Errorf(ctx, "Could not load config for project.")
		return PermissionDeniedErr(ctx)
	case err != nil:
		// The configuration attempt failed to load. This is an internal error,
		// and is safe to return because it's not contingent on the existence (or
		// lack thereof) of the project.
		return status.Error(codes.Internal, "internal server error")
	}

	// All future datastore queries are scoped to this project.
	pns := ProjectNamespace(project)
	nc, err := info.Namespace(ctx, pns)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"namespace":  pns,
		}.Errorf(ctx, "Failed to set namespace.")
		return status.Error(codes.Internal, "internal server error")
	}

	// Store the project config in the context to avoid fetching it again.
	nc = context.WithValue(nc, &projectConfigCtxKey, pcfg)

	*c = nc
	return nil
}

// Project returns the current project installed in the supplied Context's
// namespace.
//
// This function is called with the expectation that the Context is in a
// namespace conforming to ProjectNamespace. If this is not the case, this
// method will panic.
func Project(ctx context.Context) string {
	ns := info.GetNamespace(ctx)
	project := ProjectFromNamespace(ns)
	if project != "" {
		return project
	}
	panic(fmt.Errorf("current namespace %q does not begin with project namespace prefix (%q)", ns, ProjectNamespacePrefix))
}

// ProjectConfig returns the project-specific configuration for the
// current project as set in WithProjectNamespace.
//
// If there is no current project namespace, or if the current project has no
// configuration, config.ErrInvalidConfig will be returned.
func ProjectConfig(ctx context.Context) (*svcconfig.ProjectConfig, error) {
	cfg, _ := ctx.Value(&projectConfigCtxKey).(*svcconfig.ProjectConfig)
	if cfg == nil {
		return nil, config.ErrInvalidConfig
	}
	return cfg, nil
}
