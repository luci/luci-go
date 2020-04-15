// Copyright 2020 The LUCI Authors.
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

package realms

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
)

var (
	projectNameRe = regexp.MustCompile(`^[a-z0-9\-_]{1,100}$`)
	realmNameRe   = regexp.MustCompile(`^[a-z0-9_\.\-/]{1,400}$`)
)

// RealmNameScope specifies how realm names are scoped for ValidateRealmName.
type RealmNameScope string

const (
	// GlobalScope indicates the realm name is not scoped to a project.
	//
	// E.g. it is "<project>:<realm>".
	GlobalScope RealmNameScope = "global"

	// ProjectScope indicates the realm name is scoped to some project.
	//
	// E.g. it is just "<realm>" (instead of "<project>:<realm>").
	ProjectScope RealmNameScope = "project-scoped"
)

const (
	// Root is an alias for "@root".
	//
	// The root realm is implicitly included into all other realms (including
	// "@legacy"), and it is also used as a fallback when a resource points to
	// a realm that no longer exists. Without the root realm, such resources
	// become effectively inaccessible and this may be undesirable. Permissions in
	// the root realm apply to all realms in the project (current, past and
	// future), and thus the root realm should contain only administrative-level
	// bindings.
	//
	// HasPermission() automatically falls back to corresponding root realms if
	// any of the realms it receives do not exist. You still can pass a root realm
	// to HasPermission() if you specifically want to check the root realm
	// permissions.
	Root = "@root"

	// Legacy is an alias for "@legacy".
	//
	// The legacy realm should be used for legacy resources created before the
	// realms mechanism was introduced in case the service can't figure out a more
	// appropriate realm based on resource's properties. The service must clearly
	// document when and how it uses the legacy realm (if it uses it at all).
	//
	// Unlike the situation with root realms, HasPermission() has no special
	// handling of legacy realms. You should always pass them to HasPermission()
	// explicitly when checking permissions of legacy resources.
	Legacy = "@legacy"
)

// ValidateRealmName returns an error if a realm name is not valid.
//
// If `scope` is GlobalScope, `realm` is expected to have the form
// "<project>:<realm>". If `scope` is ProjectScope, `realm` is expected to have
// the form "<realm>". Any other values of `scope` cause panics.
//
// In either case "<realm>" is tested against `^[a-z0-9_\.\-/]{1,400}$` and
// compared to literals "@root" and "@legacy".
//
// When validating globally scoped names, "<project>" is tested against
// `^[a-z0-9\-_]{1,100}$` (via ValidateProjectName).
func ValidateRealmName(realm string, scope RealmNameScope) error {
	if scope == GlobalScope {
		idx := strings.IndexRune(realm, ':')
		if idx == -1 {
			return errors.Reason("bad %s realm name %q - should be <project>:<realm>", scope, realm).Err()
		}
		if err := ValidateProjectName(realm[:idx]); err != nil {
			return errors.Annotate(err, "bad %s realm name %q", scope, realm).Err()
		}
		realm = realm[idx+1:]
	} else if scope != ProjectScope {
		panic(fmt.Sprintf("invalid RealmNameScope %q", scope))
	}

	if realm != Root && realm != Legacy && !realmNameRe.MatchString(realm) {
		return errors.Reason("bad %s realm name %q - the realm name should match %q or be %q or %q", scope, realm, realmNameRe, Root, Legacy).Err()
	}

	return nil
}

// ValidateProjectName returns an error if the project name is not valid.
//
// It should match `^[a-z0-9\-_]{1,100}$`.
func ValidateProjectName(project string) error {
	if !projectNameRe.MatchString(project) {
		return errors.Reason("bad project name %q - should match %q", project, projectNameRe).Err()
	}
	return nil
}

// Split splits a global realm name "<project>:<realm>" into its components.
//
// Panics if `global` doesn't have ":". Doesn't validate the resulting
// components. If this is a concern, use ValidateRealmName explicitly.
func Split(global string) (project, realm string) {
	idx := strings.IndexRune(global, ':')
	if idx == -1 {
		panic(fmt.Sprintf("bad realm name %q - should be <project>:<realm>", global))
	}
	return global[:idx], global[idx+1:]
}

// Join returns "<project>:<realm>".
//
// Doesn't validate the result. If this is a concern, use ValidateRealmName
// explicitly.
func Join(project, realm string) (global string) {
	return project + ":" + realm
}
