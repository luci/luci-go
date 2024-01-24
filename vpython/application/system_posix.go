// Copyright 2017 The LUCI Authors.
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

//go:build unix
// +build unix

package application

import (
	"context"
	"os"
	"syscall"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

func execImpl(c context.Context, argv []string, env environ.Env, dir string) error {
	// Change directory.
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return errors.Annotate(err, "failed to chdir to %q", dir).Err()
		}
	}

	if err := syscall.Exec(argv[0], argv, env.Sorted()); err != nil {
		panic(errors.Annotate(err, "failed to execve %q", argv[0]).Err())
	}
	panic("must not return")
}
