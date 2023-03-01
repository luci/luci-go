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

package model

import (
	"context"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
)

// taskRequestIDMask is xored with TaskRequest entity ID.
const taskRequestIDMask = 0x7fffffffffffffff

// TaskRequestKey returns TaskRequest entity key given a task ID string.
//
// The task ID is something that looks like "60b2ed0a43023110", it is either
// a "packed TaskResultSummary key" (when ends with 0) or "a packed
// TaskRunResult key" (when ends with non-0).
func TaskRequestKey(ctx context.Context, taskID string) (*datastore.Key, error) {
	if err := checkIsHex(taskID, 2); err != nil {
		return nil, errors.Annotate(err, "bad task ID").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	// Chop the suffix byte. It is TaskRunResult index, we don't care about it.
	num, err := strconv.ParseInt(taskID[:len(taskID)-1], 16, 64)
	if err != nil {
		return nil, errors.Annotate(err, "bad task ID").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	return datastore.NewKey(ctx, "TaskRequest", "", num^taskRequestIDMask, nil), nil
}
