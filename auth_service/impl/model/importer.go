// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// Imports groups from some external tar.gz bundle or plain text list.
// External URL should serve *.tar.gz file with the following file structure:
//   <external group system name>/<group name>:
//     userid
//     userid
//     ...

// For example ldap.tar.gz may look like:
//   ldap/trusted-users:
//     jane
//     joe
//     ...
//   ldap/all:
//     jane
//     joe
//     ...

// Each tarball may have groups from multiple external systems, but groups from
// some external system must not be split between multiple tarballs. When importer
// sees <external group system name>/* in a tarball, it modifies group list from
// that system on the server to match group list in the tarball _exactly_,
// including removal of groups that are on the server, but no longer present in
// the tarball.

// Plain list format should have one userid per line and can only describe a single
// group in a single system. Such groups will be added to 'external/*' groups
// namespace. Removing such group from importer config will remove it from
// service too.

// The service can also be configured to accept tarball uploads (instead of
// fetching them). Fetched and uploaded tarballs are handled in the exact same way,
// in particular all caveats related to external group system names apply.

// GroupImporterConfig is a singleton entity that contains the contents of the imports.cfg file.
type GroupImporterConfig struct {
	Kind string `gae:"$kind,GroupImporterConfig"`
	ID   string `gae:"$id,config"`

	// ConfigProto is the plaintext copy of the config found at imports.cfg.
	ConfigProto string `gae:"config_proto"`

	// ConfigRevision is revision version of the config found at imports.cfg.
	ConfigRevision []byte `gae:"config_revision"`

	// ModifiedBy is the email of the user who modified the cfg.
	ModifiedBy string `gae:"modified_by"`

	// ModifiedTS is the time when this entity was last modified.
	ModifiedTS time.Time `gae:"modified_ts"`
}

// GetGroupImporterConfig fetches the GroupImporterConfig entity from the datastore.
//
//	Returns GroupImporterConfig entity if present.
//	Returns datastore.ErrNoSuchEntity if the entity is not present.
//	Returns annotated error for all other errors.
func GetGroupImporterConfig(ctx context.Context) (*GroupImporterConfig, error) {
	groupsCfg := &GroupImporterConfig{
		Kind: "GroupImporterConfig",
		ID:   "config",
	}

	switch err := datastore.Get(ctx, groupsCfg); {
	case err == nil:
		return groupsCfg, nil
	case err == datastore.ErrNoSuchEntity:
		return nil, err
	default:
		return nil, errors.Annotate(err, "error getting GroupImporterConfig").Err()
	}
}

func loadGroupFile(identities string, domain string) ([]identity.Identity, error) {
	members := make(map[identity.Identity]bool)
	memsSplit := strings.Split(identities, "\n")
	for _, uid := range memsSplit {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		var d string
		if domain == "" {
			d = uid
		} else {
			d = domain
		}
		emailIdent, err := identity.MakeIdentity(fmt.Sprintf("user:%s@%s", uid, d))
		if err != nil {
			return nil, err
		}
		members[emailIdent] = true
	}

	membersSorted := make([]identity.Identity, 0, len(members))
	for mem := range members {
		membersSorted = append(membersSorted, mem)
	}
	sort.Slice(membersSorted, func(i, j int) bool {
		return membersSorted[i].Value() < membersSorted[j].Value()
	})

	return membersSorted, nil
}
