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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/auth/identity"
	log "go.chromium.org/luci/common/logging"
	cfglib "go.chromium.org/luci/config"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"

	"google.golang.org/grpc/codes"
)

// NamespaceAccessType specifies the type of namespace access that is being
// requested for WithProjectNamespace.
type NamespaceAccessType int

const (
	// NamespaceAccessNoAuth grants unconditional access to a project's namespace.
	// This bypasses all ACL checks, and must only be used by service endpoints
	// that explicitly apply ACLs elsewhere.
	NamespaceAccessNoAuth NamespaceAccessType = iota

	// NamespaceAccessAllTesting is an extension of NamespaceAccessNoAuth that,
	// in addition to doing no ACL checks, also does no project existence checks.
	//
	// This must ONLY be used for testing.
	NamespaceAccessAllTesting

	// NamespaceAccessREAD enforces READ permission access to a project's
	// namespace.
	NamespaceAccessREAD

	// NamespaceAccessWRITE enforces WRITE permission access to a project's
	// namespace.
	NamespaceAccessWRITE
)

var (
	configProviderKey = "LogDog ConfigProvider"
)

// WithConfigProvider installs the supplied ConfigProvider instance into a
// Context.
func WithConfigProvider(c context.Context, s ConfigProvider) context.Context {
	return context.WithValue(c, &configProviderKey, s)
}

// GetConfigProvider gets the ConfigProvider instance installed in the supplied
// Context.
//
// If no Services has been installed, it will panic.
func GetConfigProvider(c context.Context) ConfigProvider {
	return c.Value(&configProviderKey).(ConfigProvider)
}

// WithStream performs all ACL checks on a stream, including:
//
//	- Project Namespace check
//	- Logs purged check
//
// This returns the stream metadata, and a new context tied to the project.
// The new context can be used to fetch the LogStreamState from the datastore.
// The returned error is a gRPC error, parsable with grpcutil.
func WithStream(c context.Context, project types.ProjectName, path types.StreamPath, at NamespaceAccessType) (context.Context, *LogStream, error) {
	nc := c
	// Do the project namespace check.  If successful, the namespace is installed
	// into the resulting context.
	if err := WithProjectNamespace(&nc, project, at); err != nil {
		return c, nil, err
	}
	// Check to see if the log is purged.
	ls := &LogStream{ID: LogStreamID(path)}
	switch err := datastore.Get(nc, ls); {
	case datastore.IsErrNoSuchEntity(err):
		err = ErrPathNotFound
		fallthrough
	case err != nil:
		return c, nil, err
	}
	// If this log entry is Purged and we're not admin, pretend it doesn't exist.
	if ls.Purged {
		if authErr := IsAdminUser(c); authErr != nil {
			return c, nil, ErrPathNotFound
		}
	}
	// Everything is fine.
	return nc, ls, nil
}

// WithProjectNamespace sets the current namespace to the project name.
//
// It will return a user-facing wrapped gRPC error on failure:
//	- InvalidArgument if the project name is invalid.
//	- If the project exists, then
//	  - nil, if the user has the requested access.
//	  - Unauthenticated if the user does not have the requested access, but is
//	    also not authenticated. This lets them know they should try again after
//	    authenticating.
//	  - PermissionDenied if the user does not have the requested access.
//	- PermissionDenied if the project doesn't exist.
//	- Internal if an internal error occurred.
func WithProjectNamespace(c *context.Context, project types.ProjectName, at NamespaceAccessType) error {
	ctx := *c

	if err := project.Validate(); err != nil {
		log.WithError(err).Errorf(ctx, "Project name is invalid.")
		return grpcutil.Errf(codes.InvalidArgument, "Project name is invalid: %s", err)
	}

	// Return gRPC error for when the user is denied access and does not have READ
	// access. Returns either Unauthenticated if the user is not authenticated
	// or PermissionDenied if the user is authenticated.
	getAccessDeniedError := func() error {
		if id := auth.CurrentIdentity(ctx); id.Kind() == identity.Anonymous {
			return grpcutil.Unauthenticated
		}

		// Deny the existence of the project.
		return grpcutil.Errf(codes.PermissionDenied,
			"The project is invalid, or you do not have permission to access it.")
	}

	// Returns the project config, or "read denied" error if the project does not
	// exist.
	getProjectConfig := func() (*svcconfig.ProjectConfig, error) {
		pcfg, err := GetConfigProvider(ctx).ProjectConfig(ctx, project)
		switch err {
		case nil:
			// Successfully loaded project config.
			return pcfg, nil

		case cfglib.ErrNoConfig, config.ErrInvalidConfig:
			// If the configuration request was valid, but no configuration could be
			// loaded, treat this as the user not having READ access to the project.
			// Otherwise, the user could use this error response to confirm a
			// project's existence.
			log.Fields{
				log.ErrorKey: err,
				"project":    project,
			}.Errorf(ctx, "Could not load config for project.")
			return nil, getAccessDeniedError()

		default:
			// The configuration attempt failed to load. This is an internal error,
			// and is safe to return because it's not contingent on the existence (or
			// lack thereof) of the project.
			return nil, grpcutil.Internal
		}
	}

	// Validate that the current user has the requested access.
	switch at {
	case NamespaceAccessNoAuth:
		// Assert that the project exists and has a configuration.
		if _, err := getProjectConfig(); err != nil {
			return err
		}

	case NamespaceAccessAllTesting:
		// Sanity check: this should only be used on development instances.
		if !info.IsDevAppServer(ctx) {
			panic("Testing access requested on non-development instance.")
		}
		break

	case NamespaceAccessREAD:
		// Assert that the current user has READ access.
		pcfg, err := getProjectConfig()
		if err != nil {
			return err
		}

		if err := IsProjectReader(*c, pcfg); err != nil {
			log.WithError(err).Warningf(*c, "User denied READ access to requested project.")
			return getAccessDeniedError()
		}

	case NamespaceAccessWRITE:
		// Assert that the current user has WRITE access.
		pcfg, err := getProjectConfig()
		if err != nil {
			return err
		}

		if err := IsProjectWriter(*c, pcfg); err != nil {
			log.WithError(err).Errorf(*c, "User denied WRITE access to requested project.")
			return getAccessDeniedError()
		}

	default:
		panic(fmt.Errorf("unknown access type: %v", at))
	}

	pns := ProjectNamespace(project)
	nc, err := info.Namespace(ctx, pns)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"namespace":  pns,
		}.Errorf(ctx, "Failed to set namespace.")
		return grpcutil.Internal
	}

	*c = nc
	return nil
}

// Project returns the current project installed in the supplied Context's
// namespace.
//
// This function is called with the expectation that the Context is in a
// namespace conforming to ProjectNamespace. If this is not the case, this
// method will panic.
func Project(c context.Context) types.ProjectName {
	ns := info.GetNamespace(c)
	project := ProjectFromNamespace(ns)
	if project != "" {
		return project
	}
	panic(fmt.Errorf("current namespace %q does not begin with project namespace prefix (%q)", ns, projectNamespacePrefix))
}
