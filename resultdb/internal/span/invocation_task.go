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

package span

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/logging"
)

// ResetInvocationTasks is used by finalizeInvocation() to set ProcessAfter
// of an invocation's tasks.
func ResetInvocationTasks(ctx context.Context, txn *spanner.ReadWriteTransaction, id InvocationID, processAfter time.Time) error {
	st := spanner.NewStatement(`
		UPDATE InvocationTasks
		SET ProcessAfter = @processAfter
		WHERE InvocationId = @invocationId AND ResetOnFinalize
	`)
	st.Params = ToSpannerMap(map[string]interface{}{
		"invocationId": id,
		"processAfter": processAfter,
	})
	updatedRowCount, err := txn.Update(ctx, st)
	if err != nil {
		return err
	}
	logging.Infof(ctx, "Reset %d invocation tasks for invocation %s", updatedRowCount, id)
	return nil
}
