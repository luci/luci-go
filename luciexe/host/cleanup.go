// Copyright 2019 The LUCI Authors.
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

package host

import (
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

type cleanupFn func() error
type cleanupSlice []cleanupFn

func (c cleanupSlice) run() {
	merr := errors.NewLazyMultiError(len(c))
	for i := len(c) - 1; i >= 0; i-- {
		merr.Assign(i, c[i]())
	}
	if err := merr.Get(); err != nil {
		panic(err)
	}
}

func (c *cleanupSlice) add(cleanup cleanupSlice, err error) error {
	*c = append(*c, cleanup...)
	return err
}

func restoreEnv() cleanupFn {
	origEnv := environ.System()
	return func() error {
		os.Clearenv()
		return errors.Annotate(origEnv.Iter(os.Setenv), "restoring original env").Err()
	}
}
