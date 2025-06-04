// Copyright 2023 The LUCI Authors.
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

package common

import (
	"net/url"
	"strings"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
)

// ValidateGitilesLocation validate cfgcommonpb.GitilesLocation proto.
func ValidateGitilesLocation(loc *cfgcommonpb.GitilesLocation) error {
	switch {
	case loc == nil:
		return errors.New("not specified")
	case !strings.HasPrefix(loc.GetRef(), "refs/"):
		return errors.New("ref must start with 'refs/'")
	case strings.HasPrefix(loc.GetPath(), "/"):
		return errors.New("path must not start with '/'")
	default:
		return errors.WrapIf(validateGitilesRepo(loc.GetRepo()), "repo")
	}
}

func validateGitilesRepo(repo string) error {
	if repo == "" {
		return errors.New("not specified")
	}
	switch u, err := url.Parse(repo); {
	case err != nil:
		return errors.Fmt("invalid repo url: %w", err)
	case strings.HasPrefix(u.Path, "/a/"):
		return errors.New("must not have '/a/' prefix of a path component")
	case strings.HasSuffix(u.Path, ".git"):
		return errors.New("must not end with '.git'")
	case strings.HasSuffix(u.Path, "/"):
		return errors.New("must not end with '/'")
	default:
		return gitiles.ValidateRepoURL(repo)
	}
}
