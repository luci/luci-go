// Copyright 2018 The LUCI Authors.
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

// Package gitacls implements read ACLs for Git/Gerrit data.
package gitacls

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"

	configpb "go.chromium.org/luci/milo/proto/config"
)

// FromConfig returns ACLs if config is valid.
func FromConfig(c context.Context, cfg []*configpb.Settings_SourceAcls) (*ACLs, error) {
	ctx := validation.Context{Context: c}
	ACLs := &ACLs{}
	ACLs.load(&ctx, cfg)
	if err := ctx.Finalize(); err != nil {
		return nil, err
	}
	return ACLs, nil
}

// ValidateConfig passes all validation errors through validation context.
func ValidateConfig(ctx *validation.Context, cfg []*configpb.Settings_SourceAcls) {
	var a ACLs
	a.load(ctx, cfg)
}

// ACLs define readers for git repositories and Gerrits CLs.
type ACLs struct {
	hosts map[string]*hostACLs // immutable once constructed.
}

// IsAllowed returns whether current identity has been granted read access to
// given Git/Gerrit project.
func (a *ACLs) IsAllowed(c context.Context, host, project string) (bool, error) {
	// Convert Gerrit to Git hosts.
	if strings.HasSuffix(host, "-review.googlesource.com") {
		host = strings.TrimSuffix(host, "-review.googlesource.com") + ".googlesource.com"
	}
	hacls, configured := a.hosts[host]
	if !configured {
		return false, nil
	}
	if pacls, projKnown := hacls.projects[project]; projKnown {
		return a.belongsTo(c, hacls.readers, pacls.readers)
	}
	return a.belongsTo(c, hacls.readers)
}

// private implementation.

func (a *ACLs) belongsTo(c context.Context, readerGroups ...[]string) (bool, error) {
	currentIdentity := auth.CurrentIdentity(c)
	groups := []string{}
	for _, readers := range readerGroups {
		for _, r := range readers {
			switch {
			case strings.HasPrefix(r, "group:"):
				// auth.IsMember expects group names w/o "group:" prefix.
				groups = append(groups, r[len("group:"):])
			case currentIdentity == identity.Identity(r):
				return true, nil
			}
		}
	}
	isMember, err := auth.IsMember(c, groups...)
	if err != nil {
		return false, transient.Tag.Apply(err)
	}
	return isMember, nil
}

type itemACLs struct {
	definedIn int // used for validation only.
	readers   []string
}

type hostACLs struct {
	itemACLs
	projects map[string]*itemACLs // key is project name starting with '/'.
}

// load validates and loads ACLs from config.
func (a *ACLs) load(ctx *validation.Context, cfg []*configpb.Settings_SourceAcls) {
	a.hosts = map[string]*hostACLs{}
	for i, group := range cfg {
		ctx.Enter("source_acls #%d", i)

		if len(group.Readers) == 0 {
			ctx.Errorf("at least 1 reader required")
		}
		if len(group.Hosts)+len(group.Projects) == 0 {
			ctx.Errorf("at least 1 host or project required")
		}

		gReaders := a.loadReaders(ctx, group.Readers)
		for _, hostURL := range group.Hosts {
			ctx.Enter("host %q", hostURL)
			a.loadHost(ctx, i, hostURL, gReaders)
			ctx.Exit()
		}
		for _, projectURL := range group.Projects {
			ctx.Enter("project %q", projectURL)
			a.loadProject(ctx, i, projectURL, gReaders)
			ctx.Exit()
		}
		ctx.Exit()
	}
}

// loadReaders validates readers and returns slice of valid identities.
func (a *ACLs) loadReaders(ctx *validation.Context, readers []string) []string {
	known := stringset.New(len(readers))
	for _, r := range readers {
		id := r
		switch {
		case strings.HasPrefix(r, "group:"):
			if r[len("group:"):] == "" {
				ctx.Errorf("invalid readers %q: needs a group name", r)
			}
		case !strings.ContainsRune(r, ':'):
			id = "user:" + r
			fallthrough
		default:
			if _, err := identity.MakeIdentity(id); err != nil {
				ctx.Error(errors.Fmt("invalid readers %q: %w", r, err))
				continue
			}
		}
		if !known.Add(id) {
			ctx.Errorf("duplicate readers %q", r)
		}
	}
	res := known.ToSlice()
	sort.Strings(res)
	return res
}

func (a *ACLs) loadHost(ctx *validation.Context, blockID int, hostURL string, readers []string) {
	u, valid := validateURL(ctx, hostURL)
	if !valid && u == nil {
		return // Can't validate any more.
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path != "" || u.Fragment != "" {
		ctx.Errorf("shouldn't have path or fragment components")
		valid = false
	}
	switch ha, dup := a.hosts[u.Host]; {
	case dup && ha.definedIn > -1:
		ctx.Errorf("has already been defined in source_acl #%d", ha.definedIn)
	case !valid:
		return
	case dup:
		ha.definedIn = blockID
		ha.readers = readers
	default:
		a.hosts[u.Host] = &hostACLs{itemACLs: itemACLs{definedIn: blockID, readers: readers}}
	}
}
func (a *ACLs) loadProject(ctx *validation.Context, blockID int, projectURL string, readers []string) {
	u, valid := validateURL(ctx, projectURL)
	if !valid && u == nil {
		return // Can't validate any more.
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "" {
		ctx.Errorf("should not be just a host")
		valid = false
	}
	if strings.HasPrefix(u.Path, "/a/") {
		ctx.Errorf("should not contain '/a' path prefix")
		valid = false
	}
	if strings.HasSuffix(u.Path, ".git") {
		ctx.Errorf("should not contain '.git'")
		valid = false
	}
	if u.Fragment != "" {
		ctx.Errorf("shouldn't have fragment components")
		valid = false
	}
	if !valid {
		return
	}

	u.Path = u.Path[1:] // trim starting '/'.
	hACLs, knownHost := a.hosts[u.Host]
	switch {
	case knownHost && hACLs.definedIn == blockID:
		ctx.Errorf("redundant because already covered by its host in the same source_acls block")
		return
	case !knownHost:
		hACLs = &hostACLs{itemACLs: itemACLs{definedIn: -1}}
		a.hosts[u.Host] = hACLs
		fallthrough
	case hACLs.projects == nil:
		// Less optimal, but more readable.
		hACLs.projects = make(map[string]*itemACLs, 1)
	}

	switch projACLs, known := hACLs.projects[u.Path]; {
	case known && projACLs.definedIn == blockID:
		ctx.Errorf("duplicate, already defined in the same source_acls block")
	case known:
		projACLs.definedIn = blockID
		projACLs.readers = stringset.NewFromSlice(append(projACLs.readers, readers...)...).ToSlice()
		sort.Strings(projACLs.readers)
	default:
		hACLs.projects[u.Path] = &itemACLs{definedIn: blockID, readers: readers}
	}
}

// validateURL returns parsed URL and whether it has passed validation.
func validateURL(ctx *validation.Context, s string) (*url.URL, bool) {
	// url.Parse considers "example.com/a/b" to be a path, so ensure a scheme.
	if !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		ctx.Error(errors.Fmt("not a valid URL: %w", err))
		return nil, false
	}
	valid := true
	if !strings.HasSuffix(u.Host, ".googlesource.com") {
		ctx.Errorf("isn't at *.googlesource.com %q", u.Host)
		valid = false
	}
	if u.Scheme != "" && u.Scheme != "https" {
		ctx.Errorf("scheme must be https")
		valid = false
	}
	if strings.HasSuffix(u.Host, "-review.googlesource.com") {
		ctx.Errorf("must not be a Gerrit host (try without '-review')")
		valid = false
	}
	return u, valid
}
