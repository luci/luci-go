// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"errors"
	"fmt"

	"github.com/luci/gae/service/info"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"golang.org/x/net/context"
)

// IsAdminUser tests whether the current user belongs to the administrative
// users group. It will return an error if the user does not.
func IsAdminUser(c context.Context) error {
	return isMember(c, func(cfg *svcconfig.Coordinator) string {
		return cfg.AdminAuthGroup
	})
}

// IsServiceUser tests whether the current user belongs to the backend services
// users group. It will return an error if the user does not.
func IsServiceUser(c context.Context) error {
	return isMember(c, func(cfg *svcconfig.Coordinator) string {
		return cfg.ServiceAuthGroup
	})
}

func isMember(c context.Context, groupNameFunc func(*svcconfig.Coordinator) string) error {
	_, cfg, err := GetServices(c).Config(c)
	if err != nil {
		return err
	}

	// On dev-appserver, the superuser has implicit group membership to
	// everything.
	if info.Get(c).IsDevAppServer() {
		if u := auth.CurrentUser(c); u.Superuser {
			log.Fields{
				"identity": u.Identity,
			}.Infof(c, "Granting superuser implicit group membership on development server.")
			return nil
		}
	}

	if cfg.Coordinator == nil {
		return errors.New("no coordinator configuration")
	}

	groupName := groupNameFunc(cfg.Coordinator)
	if groupName == "" {
		return errors.New("no auth group is configured")
	}

	is, err := auth.IsMember(c, groupName)
	if err != nil {
		return err
	}
	if !is {
		return &MembershipError{
			Identity: auth.CurrentIdentity(c),
			Group:    groupName,
		}
	}
	return nil
}

// MembershipError is an error returned by group membership checking functions
// if the current identity is not a member of the requested group.
type MembershipError struct {
	Identity identity.Identity
	Group    string
}

func (e *MembershipError) Error() string {
	return fmt.Sprintf("user %q is not a member of group %q", e.Identity, e.Group)
}

// IsMembershipError returns whether a given error is a membership error.
func IsMembershipError(e error) bool {
	_, ok := e.(*MembershipError)
	return ok
}
