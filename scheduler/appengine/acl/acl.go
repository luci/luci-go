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

package acl

import (
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/server/auth"
)

// GrantsByRole can answer questions who can READ and who OWNS the task.
type GrantsByRole struct {
	Owners  []string `gae:",noindex"`
	Readers []string `gae:",noindex"`
}

func (g *GrantsByRole) IsOwner(c context.Context) (bool, error) {
	return hasGrant(c, g.Owners, groupsAdministrators)
}

func (g *GrantsByRole) IsReader(c context.Context) (bool, error) {
	return hasGrant(c, g.Owners, g.Readers, groupsAdministrators)
}

func (g *GrantsByRole) Equal(o *GrantsByRole) bool {
	eqSlice := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	return eqSlice(g.Owners, o.Owners) && eqSlice(g.Readers, o.Readers)
}

// AclSets are parsed and indexed `AclSet` of a project.
type AclSets map[string][]*messages.Acl

// ValidateACLSets validates list of AclSet of a project and returns AclSets.
//
// Errors are returned via validation.Context.
func ValidateACLSets(ctx *validation.Context, sets []*messages.AclSet) AclSets {
	as := make(AclSets, len(sets))
	reportedDups := stringset.New(len(sets))
	for _, s := range sets {
		_, isDup := as[s.Name]
		validName := false
		switch {
		case s.Name == "":
			ctx.Errorf("missing 'name' field'")
		case !aclSetNameRe.MatchString(s.Name):
			ctx.Errorf("%q is not valid value for 'name' field", s.Name)
		case isDup:
			if reportedDups.Add(s.Name) {
				// Report only first dup.
				ctx.Errorf("aclSet name %q is not unique", s.Name)
			}
		default:
			validName = true
		}
		// record this error regardless of whether name is valid or not
		if len(s.GetAcls()) == 0 {
			ctx.Errorf("aclSet %q has no entries", s.Name)
		} else if validName {
			// add if and only if it is valid
			as[s.Name] = s.GetAcls()
		}
	}
	return as
}

// ValidateTaskACLs validates task's ACLs and returns TaskAcls.
//
// Errors are returned via validation.Context.
func ValidateTaskACLs(ctx *validation.Context, pSets AclSets, tSets []string, tAcls []*messages.Acl) *GrantsByRole {
	grantsLists := make([][]*messages.Acl, 0, 1+len(tSets))
	ctx.Enter("acls")
	validateGrants(ctx, tAcls)
	ctx.Exit()
	grantsLists = append(grantsLists, tAcls)
	ctx.Enter("acl_sets")
	for _, set := range tSets {
		if grantsList, exists := pSets[set]; exists {
			grantsLists = append(grantsLists, grantsList)
		} else {
			ctx.Errorf("referencing AclSet %q which doesn't exist", set)
		}
	}
	ctx.Exit()
	mg := mergeGrants(grantsLists...)
	if n := len(mg.Owners) + len(mg.Readers); n > maxGrantsPerJob {
		ctx.Errorf("Job or Trigger can have at most %d acls, but %d given", maxGrantsPerJob, n)
	}
	if len(mg.Owners) == 0 {
		ctx.Errorf("Job or Trigger must have OWNER acl set")
	}
	if len(mg.Readers) == 0 {
		ctx.Errorf("Job or Trigger must have READER acl set")
	}
	return mg
}

////////////////////////////////////////////////////////////////////////////////

var (
	// aclSetNameRe is used to validate AclSet Name field.
	aclSetNameRe = regexp.MustCompile(`^[0-9A-Za-z_\-\.]{1,100}$`)
	// maxGrantsPerJob is how many different grants are specified for a job.
	maxGrantsPerJob = 32

	groupsAdministrators = []string{"group:administrators"}
)

// validateGrants validates the fields of the provided grants.
//
// Errors are returned via validation.Context.
func validateGrants(ctx *validation.Context, gs []*messages.Acl) {
	for _, g := range gs {
		switch {
		case g.GetRole() != messages.Acl_OWNER && g.GetRole() != messages.Acl_READER:
			ctx.Errorf("invalid role %q", g.GetRole())
		case g.GetGrantedTo() == "":
			ctx.Errorf("missing granted_to for role %s", g.GetRole())
		case strings.HasPrefix(g.GetGrantedTo(), "group:"):
			if g.GetGrantedTo()[len("group:"):] == "" {
				ctx.Errorf("invalid granted_to %q for role %s: needs a group name", g.GetGrantedTo(), g.GetRole())
			}
		default:
			id := g.GetGrantedTo()
			if !strings.ContainsRune(g.GetGrantedTo(), ':') {
				id = "user:" + g.GetGrantedTo()
			}
			if _, err := identity.MakeIdentity(id); err != nil {
				ctx.Error(errors.Annotate(err, "invalid granted_to %q for role %s", g.GetGrantedTo(), g.GetRole()).Err())
			}
		}
	}
}

// mergeGrants merges valid grants into GrantsByRole, removing and sorting duplicates.
func mergeGrants(grantsLists ...[]*messages.Acl) *GrantsByRole {
	all := map[messages.Acl_Role]stringset.Set{
		messages.Acl_OWNER:  stringset.New(maxGrantsPerJob),
		messages.Acl_READER: stringset.New(maxGrantsPerJob),
	}
	for _, grantsList := range grantsLists {
		for _, g := range grantsList {
			all[g.GetRole()].Add(g.GetGrantedTo())
		}
	}
	sortedSlice := func(s stringset.Set) []string {
		r := s.ToSlice()
		sort.Strings(r)
		return r
	}
	return &GrantsByRole{
		Owners:  sortedSlice(all[messages.Acl_OWNER]),
		Readers: sortedSlice(all[messages.Acl_READER]),
	}
}

// hasGrant is current user is covered by any given grants.
func hasGrant(c context.Context, grantsList ...[]string) (bool, error) {
	currentIdentity := auth.CurrentIdentity(c)
	var groups []string
	for _, grants := range grantsList {
		for _, grant := range grants {
			if strings.HasPrefix(grant, "group:") {
				groups = append(groups, grant[len("group:"):])
				continue
			}
			grantedIdentity := identity.Identity(grant)
			if !strings.ContainsRune(grant, ':') {
				// Just email.
				grantedIdentity = identity.Identity("user:" + grant)
			}
			if grantedIdentity == currentIdentity {
				return true, nil
			}
		}
	}
	if isMember, err := auth.IsMember(c, groups...); err != nil {
		return false, transient.Tag.Apply(err)
	} else {
		return isMember, nil
	}
}
