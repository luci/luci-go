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
	if err := acls.load(ctx, cfg); err != nil {
		return nil, err
	}
	if err = ctx.Finalize(); err != nil {
		return nil, err
	}
	return acls, nil
}

func ValidateAclsConfig(ctx *validation.Context, cfg []*config.Settings_SourceAcls) error {
	var acls Acls
	return acls.load(ctx, cfg)
}

// Acls defines who can read git repositories and corresponding Gerrit CLs.
type Acls struct {
	hosts map[string]hostAcls
}

// private implementation.

type hostAcls struct {
	definedIn int // used for validation only.
	readers   []string
	projects  map[string]struct {
		definedIn int // used for validation only.
		readers   []string
	}
}

// load validates and loads Acls from config.
func (a *Acls) load(ctx *validation.Context, cfg []*config.Settings_SourceAcls) error {
	a.hosts = map[string]hostAcls{}
	for i, group := range cfg {
		ctx.Enter("source_acls #%d", i)
		gReaders := a.loadReaders(ctx, group.Readers)
		a.loadHosts(ctx, group.Hosts, gReaders)
		a.loadProjects(ctx, group.Projects, gReaders)
		ctx.Exit()
	}
	return nil
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

func (a *Acls) loadHosts(ctx *validation.Context, cfgHosts, readers []string) {
	for _, hostURL := range cfgHosts {
		u, err := url.Parse(hostURL)
		if err != nil {
			ctx.Error(errors.Annotate(err, "host %q is not a valid URL", hostURL).Err())
			continue
		}
		good := true // this to emit all errors about a host at once for good UX.
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Path != "" || u.Fragment != "" {
			ctx.Errorf("host %q shouldn't have path or fragment components", hostURL)
			good = false
		}
		if strings.HasSuffix(u.Host, ".googlesource.com") {
			ctx.Errorf("host %q isn't at *.googlesource.com", hostURL)
			good = false
		}
		if u.Scheme != "" && u.Scheme != "https" {
			ctx.Errorf("host %q scheme must be https or none at all (https is implied)", hostURL)
			good = false
		}
		if strings.HasSuffix(u.Host, "-review.googlesource.com") {
			ctx.Errorf("host %q must not be a Gerrit host (try without '-review'", hostURL)
			good = false
		}
	}
}
