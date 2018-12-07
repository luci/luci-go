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
)

// LinuxStrategy is a Linux-specific PlatformStrategy.
type LinuxStrategy struct {
}

// chown modifies the given path to be owned by the given user.
func (*LinuxStrategy) chown(c context.Context, path, username string) error {
	user, err := user.Lookup(username)
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(user.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(user.Gid)
	if err != nil {
		return err
	}
	return os.Chown(path, uid, gid)
}

// newStrategy returns a new Linux-specific PlatformStrategy.
func newStrategy() PlatformStrategy {
	return &LinuxStrategy{}
}
