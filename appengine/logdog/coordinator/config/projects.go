// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	configProto "github.com/luci/luci-go/common/proto/config"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"golang.org/x/net/context"
)

const maxProjectWorkers = 32

// ErrNoAccess is returned if the user has no access to the requested project.
var ErrNoAccess = errors.New("no access")

// Projects lists the registered LogDog projects.
func Projects(c context.Context) ([]string, error) {
	projects, err := config.Get(c).GetProjects()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to list 'luci-config' projects.")
		return nil, err
	}

	// TODO(dnj): Filter this list to projects with active LogDog configs, once we
	// move to project-specific configurations.

	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}

	// TODO(dnj): Restrict this by actual namespaces in datastore.
	return ids, nil
}

// AssertProjectAccess attempts to assert the current user's ability to access
// a given project.
//
// If the user cannot access the referenced project, ErrNoAccess will be
// returned. If an error occurs during checking, that error will be returned.
// The only time nil will be returned is if the check succeeded and the user was
// verified to have access to the requested project.
//
// NOTE: In the future, this will involve loading LogDog ACLs from the project's
// configuration. For now, though, since ACLs aren't implemented (yet), we will
// be content with asserting that the user has access to the base project.
func AssertProjectAccess(c context.Context, project config.ProjectName) error {
	// Get our authenticator.
	st := auth.GetState(c)
	if st == nil {
		log.Errorf(c, "No authentication state in Context.")
		return errors.New("no authentication state in context")
	}

	hasAccess, err := checkProjectAccess(c, config.Get(c), project, st)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
		}.Errorf(c, "Failed to check for project access.")
		return err
	}

	if !hasAccess {
		log.Fields{
			log.ErrorKey: err,
			"project":    project,
			"identity":   st.User().Identity,
		}.Errorf(c, "User does not have access to this project.")
		return ErrNoAccess
	}

	return nil
}

// UserProjects returns the list of luci-config projects that the current user
// has access to.
//
// It does this by listing the full set of luci-config projects, then loading
// the project configuration for each one. If the project configuration
// specifies an access restriction that the user satisfies, that project will
// be included in the list.
//
// In a production environment, most of the config accesses will be cached.
//
// If the current user is anonymous, this will still work, returning the set of
// projects that the anonymous user can access.
func UserProjects(c context.Context) ([]config.ProjectName, error) {
	// NOTE: This client has a relatively short timeout, and using WithDeadline
	// will apply to all serial calls. We may want to make getting a config client
	// instance a coordinator.Service function if this proves to be a problem.
	ci := config.Get(c)

	// Get our authenticator.
	st := auth.GetState(c)
	if st == nil {
		log.Errorf(c, "No authentication state in Context.")
		return nil, errors.New("no authentication state in context")
	}

	allProjects, err := ci.GetProjects()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to list all projects.")
		return nil, err
	}

	// In parallel, pull each project's configuration and assert access.
	access := make([]bool, len(allProjects))
	err = parallel.WorkPool(maxProjectWorkers, func(taskC chan<- func() error) {
		for i, proj := range allProjects {
			i := i
			proj := config.ProjectName(proj.ID)

			taskC <- func() error {
				hasAccess, err := checkProjectAccess(c, ci, proj, st)
				switch err {
				case nil:
					access[i] = hasAccess
					return nil

				case config.ErrNoConfig:
					// No configuration for this project, so nothing to check. Assume no
					// access.
					return nil

				default:
					log.Fields{
						log.ErrorKey: err,
						"project":    proj,
					}.Errorf(c, "Failed to check project access.")
					return err
				}
			}
		}
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Error during project access check.")
		return nil, errors.SingleError(err)
	}

	result := make([]config.ProjectName, 0, len(allProjects))
	for i, proj := range allProjects {
		if access[i] {
			result = append(result, config.ProjectName(proj.ID))
		}
	}
	return result, nil
}

func checkProjectAccess(c context.Context, ci config.Interface, proj config.ProjectName, st auth.State) (bool, error) {
	// Load the configuration for this project.
	cfg, err := ci.GetConfig(fmt.Sprintf("projects/%s", proj), "project.cfg", false)
	if err != nil {
		if err == config.ErrNoConfig {
			// If the configuration is missing, report no access.
			return false, nil
		}
		return false, err
	}

	var pcfg configProto.ProjectCfg
	if err := proto.UnmarshalText(cfg.Content, &pcfg); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    proj,
		}.Errorf(c, "Failed to unmarshal project configuration.")
		return false, err
	}

	// Vet project access using the current Authenticator state.
	id := st.User().Identity

	for _, v := range pcfg.Access {
		// Is this a group access?
		access, isGroup := trimPrefix(v, "group:")
		if isGroup {
			// Check group membership.
			isMember, err := st.DB().IsMember(c, id, access)
			if err != nil {
				return false, fmt.Errorf("failed to check access %q: %v", v, err)
			}
			if isMember {
				log.Fields{
					"project":  proj,
					"identity": id,
					"group":    access,
				}.Debugf(c, "Identity has group membership.")
				return true, nil
			}
			return false, nil
		}

		// "access" is either an e-mail or an identity. If it is an e-mail,
		// transform it into an identity.
		if idx := strings.IndexRune(access, ':'); idx < 0 {
			// Presumably an e-mail, convert e-mail to user identity.
			access = "user:" + access
		}

		if id == identity.Identity(access) {
			// Check identity.
			log.Fields{
				"project":        proj,
				"identity":       id,
				"accessIdentity": v,
			}.Debugf(c, "Identity is included.")
			return true, nil
		}
	}

	return false, nil
}

func trimPrefix(s, p string) (string, bool) {
	if strings.HasPrefix(s, p) {
		return s[len(p):], true
	}
	return s, false
}
