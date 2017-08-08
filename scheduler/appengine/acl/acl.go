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
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
	"golang.org/x/net/context"
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
	if len(g.Readers) == 0 && len(g.Owners) == 0 {
		// This is here for backwards compatiblity before ACLs were introduced.
		// If Job doesn't specify READERs nor OWNERS explicitely, everybody can read.
		// TODO(tAndrii): remove once every Job/Trigger has ACLs specified.
		return true, nil
	}
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

// ValidateAclSets validates list of AclSet of a project and returns AclSets.
func ValidateAclSets(sets []*messages.AclSet) (AclSets, error) {
	as := make(AclSets, len(sets))
	for _, s := range sets {
		if s.Name == "" {
			return nil, fmt.Errorf("missing 'name' field'")
		}
		if !aclSetNameRe.MatchString(s.Name) {
			return nil, fmt.Errorf("%q is not valid value for 'name' field", s.Name)
		}
		if _, isDup := as[s.Name]; isDup {
			return nil, fmt.Errorf("aclSet name %q is not unique", s.Name)
		}
		if len(s.GetAcls()) == 0 {
			return nil, fmt.Errorf("aclSet %q has no entries", s.Name)
		}
		as[s.Name] = s.GetAcls()
	}
	return as, nil
}

// ValidateTaskAcls validates task's ACLs and returns TaskAcls.
func ValidateTaskAcls(pSets AclSets, tSets []string, tAcls []*messages.Acl) (*GrantsByRole, error) {
	grantsLists := make([][]*messages.Acl, 0, 1+len(tSets))
	if err := validateGrants(tAcls); err != nil {
		return nil, err
	}
	grantsLists = append(grantsLists, tAcls)
	for _, set := range tSets {
		grantsList, exists := pSets[set]
		if !exists {
			return nil, fmt.Errorf("referencing AclSet '%s' which doesn't exist", set)
		}
		grantsLists = append(grantsLists, grantsList)
	}
	mg := mergeGrants(grantsLists...)
	if n := len(mg.Owners) + len(mg.Readers); n > maxGrantsPerJob {
		return nil, fmt.Errorf("Job or Trigger can have at most %d acls, but %d given", maxGrantsPerJob, n)
	}
	return mg, nil
}

////////////////////////////////////////////////////////////////////////////////

var (
	// aclSetNameRe is used to validate AclSet Name field.
	aclSetNameRe = regexp.MustCompile(`^[0-9A-Za-z_\-\.]{1,100}$`)
	// maxGrantsPerJob is how many different grants are specified for a job.
	maxGrantsPerJob = 32

	groupsAdministrators = []string{"group:administrators"}
)

func validateGrants(gs []*messages.Acl) error {
	for _, g := range gs {
		switch {
		case g.GetRole() != messages.Acl_OWNER && g.GetRole() != messages.Acl_READER:
			return fmt.Errorf("invalid role %q", g.GetRole())
		case g.GetGrantedTo() == "":
			return fmt.Errorf("missing granted_to for role %s", g.GetRole())
		case strings.HasPrefix(g.GetGrantedTo(), "group:"):
			if g.GetGrantedTo()[len("group:"):] == "" {
				return fmt.Errorf("invalid granted_to %q for role %s: needs a group name", g.GetGrantedTo(), g.GetRole())
			}
		default:
			id := g.GetGrantedTo()
			if !strings.ContainsRune(g.GetGrantedTo(), ':') {
				id = "user:" + g.GetGrantedTo()
			}
			if _, err := identity.MakeIdentity(id); err != nil {
				return errors.Annotate(err, "invalid granted_to %q for role %s", g.GetGrantedTo(), g.GetRole()).Err()
			}
		}
	}
	return nil
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
	groups := []string{}
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
