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
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"google.golang.org/protobuf/encoding/prototext"
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

var GroupNameRe = regexp.MustCompile(`^([a-z\-]+/)?[0-9a-z_\-\.@]{1,100}$`)

// GroupBundle is a map where k: groupName, v: list of identities belonging to group k.
type GroupBundle = map[string][]identity.Identity

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

// IngestTarball handles upload of tarball's specified in 'tarball_upload' config entries.
// expected to be called in an auth context of the upload PUT request.
func IngestTarball(ctx context.Context, name string, content io.Reader) (map[string]GroupBundle, error) {
	g, err := GetGroupImporterConfig(ctx)
	if err != nil {
		return nil, err
	}
	gConfigProto, err := g.ToProto()
	if err != nil {
		return nil, errors.Annotate(err, "issue getting proto from config entity").Err()
	}
	caller := auth.CurrentIdentity(ctx)
	var entry *configspb.GroupImporterConfig_TarballUploadEntry

	// make sure that tarball_upload entry we're looking for is specified in config
	for _, tbu := range gConfigProto.GetTarballUpload() {
		if tbu.Name == name {
			entry = tbu
			break
		}
	}

	if entry == nil {
		return nil, errors.New("entry not found in tarball upload names")
	}
	if !contains(caller.Email(), entry.AuthorizedUploader) {
		return nil, errors.New(fmt.Sprintf("%q is not an authorized uploader", caller.Email()))
	}

	bundles, err := loadTarball(ctx, content, entry.GetDomain(), entry.GetSystems(), entry.GetGroups())
	if err != nil {
		return nil, errors.Annotate(err, "bad tarball").Err()
	}
	return bundles, nil
}

// loadTarball unzips tarball with groups and deserializes them.
func loadTarball(ctx context.Context, content io.Reader, domain string, systems, groups []string) (map[string]GroupBundle, error) {
	// map looks like: K: system, V: { K: groupName, V: []identities }
	bundles := make(map[string]GroupBundle)
	entries, err := extractTarArchive(content)
	if err != nil {
		return nil, err
	}

	// verify system/groupname and then parse blob if valid
	for filename, fileobj := range entries {
		chunks := strings.Split(filename, "/")
		if len(chunks) != 2 || !GroupNameRe.MatchString(chunks[1]) {
			logging.Warningf(ctx, "Skipping file %s, not a valid name", filename)
			continue
		}
		if groups != nil && !contains(filename, groups) {
			continue
		}
		system := chunks[0]
		if !contains(system, systems) {
			logging.Warningf(ctx, "Skipping file %s, not allowed", filename)
			continue
		}
		identities, err := loadGroupFile(string(fileobj), domain)
		if err != nil {
			return nil, err
		}
		if _, ok := bundles[system]; !ok {
			bundles[system] = make(GroupBundle)
		}
		bundles[system][filename] = identities
	}
	return bundles, nil
}

func loadGroupFile(identities string, domain string) ([]identity.Identity, error) {
	members := make(map[identity.Identity]bool)
	memsSplit := strings.Split(identities, "\n")
	for _, uid := range memsSplit {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		var ident string
		if domain == "" {
			ident = fmt.Sprintf("user:%s", uid)
		} else {
			ident = fmt.Sprintf("user:%s@%s", uid, domain)
		}
		emailIdent, err := identity.MakeIdentity(ident)
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

// extractTarArchive unpacks a tar archive and returns a map
// of filename -> fileobj pairs.
func extractTarArchive(r io.Reader) (map[string][]byte, error) {
	entries := make(map[string][]byte)
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		fileContents, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		entries[header.Name] = fileContents
	}

	if err := gzr.Close(); err != nil {
		return nil, err
	}
	return entries, nil
}

// TODO(cjacomet): replace with slices.Contains when
// slices package isn't experimental.
func contains(key string, search []string) bool {
	for _, val := range search {
		if val == key {
			return true
		}
	}
	return false
}

// ToProto converts the GroupImporterConfig entity to the proto equivalent.
func (g *GroupImporterConfig) ToProto() (*configspb.GroupImporterConfig, error) {
	gConfig := &configspb.GroupImporterConfig{}
	if err := prototext.Unmarshal([]byte(g.ConfigProto), gConfig); err != nil {
		return nil, err
	}
	return gConfig, nil
}
