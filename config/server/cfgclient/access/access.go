// Copyright 2016 The LUCI Authors.
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

// Package access implements a config service access check against a project
// config client.
//
// Note that this is a soft check, as the true access authority is the config
// service, and this check is not hitting that service.
//
// If access is granted, this function will return nil. If access is explicitly
// denied, this will return ErrNoAccess.
//
// This is a port of the ACL implementation from the config service:
// https://chromium.googlesource.com/external/github.com/luci/luci-py/+/e3fbb1f5dafa59a2c57cf3a9fe3708f4309ab653/appengine/components/components/config/api.py
package access

import (
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/server/auth"

	"golang.org/x/net/context"
)

// ErrNoAccess is an error returned by CheckAccess if the supplied Authority
// does not have access to the supplied config set.
var ErrNoAccess = errors.New("no access")

// Check tests if a given Authority can access the named config set.
func Check(c context.Context, a backend.Authority, configSet config.Set) error {
	if a == backend.AsService {
		return nil
	}

	project := configSet.Project()
	if project == "" {
		// Not a project config set, so neither remaining Authority can access.
		return ErrNoAccess
	}

	// Root project config set, contains project.cfg file that has project ACL
	// definitions.
	projectConfigSet := config.ProjectSet(project)

	// Load the project config. We execute this RPC as the service, not the user,
	// so while this will recurse (and hopefully take advantage of the cache), it
	// will not trigger an infinite access check loop.
	var pcfg configPB.ProjectCfg
	if err := cfgclient.Get(c, cfgclient.AsService, projectConfigSet, cfgclient.ProjectConfigPath,
		textproto.Message(&pcfg), nil); err != nil {
		return errors.Annotate(err, "failed to load %q in %q",
			cfgclient.ProjectConfigPath, projectConfigSet).Err()
	}

	id := identity.AnonymousIdentity
	if a == backend.AsUser {
		id = auth.CurrentIdentity(c)
	}
	checkGroups := make([]string, 0, len(pcfg.Access))
	for _, access := range pcfg.Access {
		if group, ok := trimPrefix(access, "group:"); ok {
			// Check group membership.
			checkGroups = append(checkGroups, group)
		} else {
			// If there is no ":" in the access string, this is a user ACL.
			if strings.IndexRune(access, ':') < 0 {
				access = "user:" + access
			}
			if identity.Identity(access) == id {
				return nil
			}
		}
	}

	// No individual accesses, check groups.
	if len(checkGroups) > 0 {
		switch canAccess, err := auth.IsMember(c, checkGroups...); {
		case err != nil:
			return errors.Annotate(err, "failed to check group membership").Err()
		case canAccess:
			return nil
		}
	}
	return ErrNoAccess
}

func trimPrefix(v, pfx string) (string, bool) {
	if strings.HasPrefix(v, pfx) {
		return v[len(pfx):], true
	}
	return v, false
}
