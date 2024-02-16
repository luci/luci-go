// Copyright 2024 The LUCI Authors.
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

package gs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/configs/srvcfg/settingscfg"
)

// mockedGSClientKey is the context key to indicate using a mocked GS
// client in tests.
var mockedGSClientKey = "mock Google Storage client"

func constructReadACLs(readers stringset.Set) []storage.ACLRule {
	// Sorting the readers just makes it easier to set expected ACLs in
	// tests.
	sortedReaders := readers.ToSortedSlice()

	acls := make([]storage.ACLRule, len(readers))
	for i, reader := range sortedReaders {
		acls[i] = storage.ACLRule{
			Entity: storage.ACLEntity(fmt.Sprintf("user-%s", reader)),
			Role:   storage.RoleReader,
		}
	}
	return acls
}

// GetPath returns the sanitized Google Storage path from settings.cfg.
func GetPath(ctx context.Context) (string, error) {
	cfg, err := settingscfg.Get(ctx)
	if err != nil {
		return "", errors.Annotate(err, "error getting settings.cfg").Err()
	}

	// Allow for a single trailing slash.
	path := strings.TrimSuffix(cfg.GetAuthDbGsPath(), "/")

	return path, nil
}

// IsValidPath returns whether the given path is considered a valid
// Google Storage path, where:
//   - the path is not empty; and
//   - the path has no trailing "/", as object paths are constructed
//     assuming this.
func IsValidPath(path string) bool {
	return path != "" && !strings.HasSuffix(path, "/")
}

// UploadAuthDB uploads the signed AuthDB and AuthDBRevision to Google
// Storage.
func UploadAuthDB(ctx context.Context, signedAuthDB *protocol.SignedAuthDB, revision *protocol.AuthDBRevision, readers stringset.Set, dryRun bool) (retErr error) {
	// Skip if the GS path is invalid.
	gsPath, err := GetPath(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting GS path").Err()
	}
	if !IsValidPath(gsPath) {
		if gsPath == "" {
			// Was not configured in settings.cfg; skip upload.
			return nil
		}
		return fmt.Errorf("invalid GS path: %s", gsPath)
	}

	fileBaseName := "latest"
	if dryRun {
		fileBaseName = "V2latest"
	}

	acls := constructReadACLs(readers)

	client, err := newClient(ctx)
	if err != nil {
		return err
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	// Upload signed AuthDB.
	authDBData, err := proto.Marshal(signedAuthDB)
	if err != nil {
		return fmt.Errorf("error marshalling signed AuthDB")
	}
	authDBPath := fmt.Sprintf("%s/%s.db", gsPath, fileBaseName)
	err = client.WriteFile(ctx, authDBPath, "application/protobuf", authDBData, acls)
	if err != nil {
		return errors.Annotate(err, "failed to upload %s", authDBPath).Err()
	}

	// Upload AuthDBRevision.
	authDBRevision, err := json.Marshal(revision)
	if err != nil {
		return fmt.Errorf("error marshalling AuthDBRevision")
	}
	revPath := fmt.Sprintf("%s/%s.json", gsPath, fileBaseName)
	err = client.WriteFile(ctx, revPath, "application/json", authDBRevision, acls)
	if err != nil {
		return errors.Annotate(err, "failed to upload %s", revPath).Err()
	}

	return nil
}

func newClient(ctx context.Context) (Client, error) {
	if mockClient, ok := ctx.Value(&mockedGSClientKey).(*MockClient); ok {
		// return a mock of the Google storage client for tests.
		return mockClient, nil
	}

	client, err := NewGSClient(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "error making Google Storage client").Err()
	}

	return client, nil
}
