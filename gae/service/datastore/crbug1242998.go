// Copyright 2021 The LUCI Authors.
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

package datastore

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/logging"
)

// EnableSafeGet enables correct resetting of the GAE-exported fields
// when deserializing entity from Datastore.
//
// This is a temporary function for graceful rollout of the correct behavior to
// all existing apps. Must be done for new apps. All existing apps must migrate
// to it per https://crbug.com/1242998. The option will become the default behavior
// by January 2022.
//
// If you are migrating the existing app, consider gotchas listed in description
// of this CL https://crrev.com/c/3103793. This should not affect most apps, but
// it's recommended that you scan through all datastore.Get() calls in your
// codebase.
//
// So, you should either call this directly from your main package init(),
// or use a one liner:
//
//  import _ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"
//
// You may also want to import it in your test which run against in-memory
// Datastore.
//
// NOTE: this switch is global, so if you use a shared library, which performs
// Datastore operations, this switch will also affect the library.
//
// Panics if called after datastore Get/Query functionaly was used.
func EnableSafeGet() {
	if atomic.LoadInt32(&crbug1242998checked) == 1 {
		panic(fmt.Errorf("datastore.EnableSafeGet must be called in init() before any datastore read operations"))
	}
	atomic.StoreInt32(&crbug1242998enabled, 1)
}

// crbug1242998errorMessage is emitted to stderr and logs in test & prod.
//
// \n on both sides is to make it stand out in stderr logs,
// especially when Convey() is used.
const crbug1242998errorMessage = "\nFIXME by January 2022: please call datastore.EnableSafeGet() in your package init(). See https://crbug.com/1242998\n"

var (
	crbug1242998enabled      int32
	crbug1242998checked      int32
	crbug1242998stderrLogged sync.Once
	crbug1242998ctxLogged    sync.Once
)

func getSafeGetEnabled() bool {
	atomic.StoreInt32(&crbug1242998checked, 1)
	// TODO(crbug/1242998): make enabled the default.
	if atomic.LoadInt32(&crbug1242998enabled) == 0 {
		crbug1242998stderrLogged.Do(func() {
			fmt.Fprintf(os.Stderr, crbug1242998errorMessage)
		})
		return false
	}
	return true
}

func logErrorIfSafeGetIsDisabled(ctx context.Context) {
	if !getSafeGetEnabled() {
		crbug1242998ctxLogged.Do(func() {
			logging.Errorf(ctx, crbug1242998errorMessage)
		})
	}
}
