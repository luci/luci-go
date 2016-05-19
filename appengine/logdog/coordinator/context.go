// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"fmt"

	"github.com/luci/gae/service/info"
	luciConfig "github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

type servicesKeyType int

// WithServices installs the supplied Services instance into a Context.
func WithServices(c context.Context, s Services) context.Context {
	return context.WithValue(c, servicesKeyType(0), s)
}

// GetServices gets the Services instance installed in the supplied Context.
//
// If no Services has been installed, it will panic.
func GetServices(c context.Context) Services {
	s, ok := c.Value(servicesKeyType(0)).(Services)
	if !ok {
		panic("no Services instance is installed")
	}
	return s
}

// WithProjectNamespace sets the current namespace to the project name.
//
// It will return an error if the project name or the project's namespace is
// invalid.
//
// If the current user does not have READ permission for the project, a
// MembershipError will be returned.
func WithProjectNamespace(c *context.Context, project luciConfig.ProjectName) error {
	return withProjectNamespaceImpl(c, project, true)
}

// WithProjectNamespaceNoAuth sets the current namespace to the project name. It
// does NOT assert that the current user has project access. This should only be
// used for service functions that are not acting on behalf of a user.
//
// It will fail if the project name is invalid.
func WithProjectNamespaceNoAuth(c *context.Context, project luciConfig.ProjectName) error {
	return withProjectNamespaceImpl(c, project, false)
}

func withProjectNamespaceImpl(c *context.Context, project luciConfig.ProjectName, auth bool) error {
	ctx := *c

	// TODO(dnj): REQUIRE this to be non-empty once namespacing is mandatory.
	if project == "" {
		return nil
	}

	if err := project.Validate(); err != nil {
		log.WithError(err).Errorf(ctx, "Project name is invalid.")
		return err
	}

	// Validate the user's READ access to the named project, if authenticating.
	if auth {
		pcfg, err := GetServices(ctx).ProjectConfig(ctx, project)
		if err != nil {
			log.WithError(err).Errorf(ctx, "Failed to load project config.")
			return err
		}

		if err := IsProjectReader(ctx, pcfg); err != nil {
			log.WithError(err).Errorf(ctx, "User cannot access requested project.")
			return err
		}
	}

	pns := ProjectNamespace(project)
	nc, err := info.Get(ctx).Namespace(pns)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"namespace":  pns,
		}.Errorf(ctx, "Failed to set namespace.")
		return err
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
func Project(c context.Context) luciConfig.ProjectName {
	ns, _ := info.Get(c).GetNamespace()

	// TODO(dnj): Remove the empty namespace/project exception once we no longer
	// support that.
	if ns == "" {
		return ""
	}

	project := ProjectFromNamespace(ns)
	if project != "" {
		return project
	}
	panic(fmt.Errorf("current namespace %q does not begin with project namespace prefix (%q)", ns, projectNamespacePrefix))
}
