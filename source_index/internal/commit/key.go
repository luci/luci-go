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

package commit

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/validationutil"
)

// Key denotes the primary key for the Commits table.
type Key struct {
	// host is the gitiles host of the repository. Must be a subdomain of
	// `.googlesource.com` (e.g. chromium.googlesource.com).
	host string
	// repository is the Gitiles project of the commit (e.g. "chromium/src" part
	// in https://chromium.googlesource.com/chromium/src/+/main).
	repository string
	// commitHash is the full hex sha1 of the commit in lowercase.
	commitHash string
}

// CommitHash returns the commit hash.
func (k Key) CommitHash() string {
	return k.commitHash
}

// NewKey creates a new Commit key.
func NewKey(host, repository, commitHash string) (Key, error) {
	if err := gitiles.ValidateRepoHost(host); err != nil {
		return Key{}, errors.Annotate(err, "invalid host").Err()
	}

	if err := validationutil.ValidateRepoName(repository); err != nil {
		return Key{}, errors.Annotate(err, "invalid repository").Err()
	}

	commitHash = strings.ToLower(commitHash)
	if err := validationutil.ValidateCommitHash(commitHash); err != nil {
		return Key{}, errors.Annotate(err, "invalid commit hash").Err()
	}

	return Key{
		host:       host,
		repository: repository,
		commitHash: commitHash,
	}, nil
}

// URL returns the URL for the commit.
func (k Key) URL() string {
	return fmt.Sprintf("https://%s/%s/+/%s", k.host, k.repository, k.commitHash)
}

// spannerKey returns the spanner key for the commit key.
func (k Key) spannerKey() spanner.Key {
	return spanner.Key{k.host, k.repository, k.commitHash}
}

// Exists checks whether the commit key exists in the database.
func Exists(ctx context.Context, k Key) (bool, error) {
	_, err := span.ReadRow(ctx, "Commits", k.spannerKey(), []string{"CommitHash"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return false, nil
		}
		return false, errors.Annotate(err, "reading Commits table row").Err()
	}

	return true, nil
}
