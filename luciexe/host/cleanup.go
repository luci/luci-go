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
	"context"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
)

type cleanupFn func() error
type cleanupItem struct {
	name string
	cb   cleanupFn
}
type cleanupSlice []cleanupItem

func (c cleanupSlice) run(ctx context.Context) {
	merr := errors.NewLazyMultiError(len(c))
	for i := len(c) - 1; i >= 0; i-- {
		itm := c[i]
		logging.Infof(ctx, "running cleanup %q", itm.name)
		err := itm.cb()
		if merr.Assign(i, err) {
			logging.WithError(err).Errorf(ctx, "cleanup %q failed", itm.name)
		} else {
			logging.Infof(ctx, "cleanup %q succeeded", itm.name)
		}
	}
	if err := merr.Get(); err != nil {
		panic(err)
	}
}

func (c *cleanupSlice) concat(cleanup cleanupSlice, err error) error {
	*c = append(*c, cleanup...)
	return err
}

func (c *cleanupSlice) add(name string, cb cleanupFn) {
	*c = append(*c, cleanupItem{name, cb})
}

func restoreEnv() cleanupFn {
	origEnv := environ.System()
	return func() error {
		os.Clearenv()
		if err := origEnv.Iter(os.Setenv); err != nil {
			return errors.Annotate(err, "restoring original env").Err()
		}
		return nil
	}
}
