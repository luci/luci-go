// Copyright 2015 The LUCI Authors.
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

package store

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/tsmon/store/storetest"
	"go.chromium.org/luci/common/tsmon/target"
)

func TestInMemory(t *testing.T) {
	ctx := context.Background()
	storetest.RunStoreImplementationTests(t, ctx, storetest.TestOptions{
		Factory: func() storetest.Store {
			return NewInMemory(&target.Task{ServiceName: "default target"})
		},
		RegistrationFinished: func(storetest.Store) {},
		GetNumRegisteredMetrics: func(s storetest.Store) int {
			return len(s.(*inMemoryStore).data)
		},
	})
}
