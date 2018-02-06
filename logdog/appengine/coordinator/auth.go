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
	"fmt"
	"strings"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/auth/identity"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/server/auth"
	"golang.org/x/net/context"
)

// IsAdminUser tests whether the current user belongs to the administrative
// users group.
//
// If the user is not, a MembershipError will be returned.
func IsAdminUser(c context.Context) error {
	cfg, err := GetConfigProvider(c).Config(c)
	if err != nil {
		return err
	}
	return checkMember(c, cfg.Coordinator.AdminAuthGroup)
}

// IsServiceUser tests whether the current user belongs to the backend services
// users group.
//
// If the user is not, a MembershipError will be returned.
func IsServiceUser(c context.Context) error {
	cfg, err := GetConfigProvider(c).Config(c)
	if err != nil {
		return err
	}
	return checkMember(c, cfg.Coordinator.ServiceAuthGroup)
}

// IsProjectReader tests whether the current user belongs to one of the
// project's declared reader groups.
//
// If the user is not, a MembershipError will be returned.
func IsProjectReader(c context.Context, pcfg *svcconfig.ProjectConfig) error {
	return checkMember(c, pcfg.ReaderAuthGroups...)
}

// IsProjectWriter tests whether the current user belongs to one of the
// project's declared writer groups.
//
// If the user is not a member of any of the groups, a MembershipError will be
// returned.
func IsProjectWriter(c context.Context, pcfg *svcconfig.ProjectConfig) error {
	return checkMember(c, pcfg.WriterAuthGroups...)
}

func checkMember(c context.Context, groups ...string) error {
	// On dev-appserver, the superuser has implicit group membership to
	// everything.
	if info.IsDevAppServer(c) {
		if u := auth.CurrentUser(c); u.Superuser {
			log.Fields{
				"identity": u.Identity,
				"groups":   groups,
			}.Infof(c, "Granting superuser implicit group membership on development server.")
			return nil
		}
	}

	id := auth.CurrentIdentity(c)
	is, err := auth.IsMember(c, groups...)
	if err != nil {
		return err
	}
	if is {
		log.Fields{
			"identity": id,
			"group":    groups,
		}.Debugf(c, "User access granted.")
		return nil
	}

	return &MembershipError{
		Identity: id,
		Groups:   groups,
	}
}

// MembershipError is an error returned by group membership checking functions
// if the current identity is not a member of the requested group.
type MembershipError struct {
	Identity identity.Identity
	Groups   []string
}

func (e *MembershipError) Error() string {
	return fmt.Sprintf("user %q is not a member of [%s]", e.Identity, strings.Join(e.Groups, ", "))
}

// IsMembershipError returns whether a given error is a membership error.
func IsMembershipError(e error) bool {
	_, ok := e.(*MembershipError)
	return ok
}
