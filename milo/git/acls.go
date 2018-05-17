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

package git

import (
	"net/url"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/milo/api/config"
)

func AclsFromConfig(cfg []*config.Settings_SourceAcls) (*Acls, error) {
	acls := &Acls{}
	var ctx validation.Context
	acls.load(&ctx, cfg)
	if err := ctx.Finalize(); err != nil {
		return nil, err
	}
	return acls, nil
}

func ValidateAclsConfig(ctx *validation.Context, cfg []*config.Settings_SourceAcls) {
	var acls Acls
	acls.load(ctx, cfg)
}

// Acls defines who can read git repositories and corresponding Gerrit CLs.
type Acls struct {
	hosts map[string]*hostAcls
}

// private implementation.

type itemAcls struct {
	definedIn int // used for validation only.
	readers   []string
}

type hostAcls struct {
	itemAcls
	projects map[string]itemAcls // key is project name starting with '/'.
}

// load validates and loads Acls from config.
func (a *Acls) load(ctx *validation.Context, cfg []*config.Settings_SourceAcls) {
	a.hosts = map[string]*hostAcls{}
	for i, group := range cfg {
		ctx.Enter("source_acls #%d", i)
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
func (a *Acls) loadReaders(ctx *validation.Context, cfgReaders []string) []string {
	known := stringset.New(len(cfgReaders))
	for _, r := range cfgReaders {
		id := r
		if !strings.ContainsRune(r, ':') {
			id = "user:" + r
		}
		if _, err := identity.MakeIdentity(id); err != nil {
			ctx.Error(errors.Annotate(err, "invalid readers %q", r).Err())
			continue
		}
		if !known.Add(id) {
			ctx.Errorf("duplicate readers %q", r)
		}
	}
	return known.ToSlice()
}

func (a *Acls) loadHost(ctx *validation.Context, blockId int, hostURL string, readers []string) {
	u, valid := validateURL(ctx, hostURL)
	if !valid && u == nil {
		return // Can't validate any more.
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path != "" || u.Fragment != "" {
		ctx.Errorf("shouldn't have path or fragment components")
		valid = false
	}
	if ha, dup := a.hosts[u.Host]; dup {
		ctx.Errorf("has already been defined in source_acl #%d", ha.definedIn)
		valid = false
	}
	if valid {
		a.hosts[u.Host] = &hostAcls{itemAcls: itemAcls{definedIn: blockId, readers: readers}}
	}
}
func (a *Acls) loadProject(ctx *validation.Context, blockId int, projectURL string, readers []string) {
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

	// u.Path at this point starts with '/' and will be key in projects map.
	hacls, knownHost := a.hosts[u.Host]
	switch {
	case knownHost && hacls.definedIn == blockId:
		ctx.Errorf("redundant because already covered by its host in the same source_acls block")
		return
	case !knownHost:
		hacls = &hostAcls{itemAcls: itemAcls{definedIn: -1}}
		a.hosts[u.Host] = hacls
		fallthrough
	case hacls.projects == nil:
		// Less optimal, but more readable.
		hacls.projects = make(map[string]itemAcls, 1)
	}

	switch projAcls, dup := hacls.projects[u.Path]; {
	case dup && projAcls.definedIn == blockId:
		ctx.Errorf("duplicate, already defined in the same source_acls block")
	case dup: // projAcls.definedIn != blockId
		ctx.Errorf("duplicate, already defined in the #%d source_acls block")
	default:
		hacls.projects[u.Path] = itemAcls{definedIn: blockId, readers: readers}
	}
}

// validateURL returns parsed URL and whether it has passed validation.
func validateURL(ctx *validation.Context, s string) (*url.URL, bool) {
	u, err := url.Parse(s)
	if err != nil {
		ctx.Error(errors.Annotate(err, "not a valid URL").Err())
		return nil, false
	}
	valid := true
	if strings.HasSuffix(u.Host, ".googlesource.com") {
		ctx.Errorf("isn't at *.googlesource.com")
		valid = false
	}
	if u.Scheme != "" && u.Scheme != "https" {
		ctx.Errorf("scheme must be https or none at all (https is implied)")
		valid = false
	}
	if strings.HasSuffix(u.Host, "-review.googlesource.com") {
		ctx.Errorf("must not be a Gerrit host (try without '-review'")
		valid = false
	}
	return u, valid
}
