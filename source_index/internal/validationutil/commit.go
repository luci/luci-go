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

package validationutil

import (
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"
)

var (
	repoHostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])*\.googlesource\.com$`)

	// The GoB repository naming restriction doesn't appear to be well documented.
	// The following is derived from
	//  * GitHub repository naming restriction, and
	//  * observation of existing repositories on various gitiles hosts.
	repositoryRE     = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*$`)
	repositoryMinLen = 1
	repositoryMaxLen = 100

	sha1RE     = regexp.MustCompile(`^[0-9a-f]*$`)
	sha1MinLen = 40
	sha1MaxLen = 40
)

// ValidateRepoHost checks whether the specified host is a valid Gitiles host.
func ValidateRepoHost(host string) error {
	if !repoHostRE.MatchString(host) {
		return errors.Fmt("hostname %s is not a valid Gitiles host", host)
	}
	return nil
}

// ValidateRepoName checks whether the specified name is a valid repository
// name.
func ValidateRepoName(name string) error {
	return validate.MatchReWithLength(repositoryRE, repositoryMinLen, repositoryMaxLen, name)
}

// ValidateCommitHash checks whether the specified hash is a valid sha-1 hash.
func ValidateCommitHash(hash string) error {
	return validate.MatchReWithLength(sha1RE, sha1MinLen, sha1MaxLen, hash)
}
