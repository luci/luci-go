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

package main

import (
	"context"
	"os"
	"os/user"
	"strconv"

	"go.chromium.org/luci/common/errors"
)

// LinuxStrategy is a Linux-specific partial PlatformStrategy.
// Does not fully implement PlatformStrategy.
type LinuxStrategy struct {
}

// chown modifies the given path to be owned by the given user.
// Implements PlatformStrategy.
func (*LinuxStrategy) chown(c context.Context, path, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return errors.Fmt("failed to look up local user %q: %w", username, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return errors.Fmt("failed to get uid for user %q: %w", username, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return errors.Fmt("failed to get gid for user %q: %w", username, err)
	}
	return os.Chown(path, uid, gid)
}
