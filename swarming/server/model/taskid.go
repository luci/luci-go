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
	"fmt"
	"strconv"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
)

// taskRequestIDMask is xored with TaskRequest entity ID.
const taskRequestIDMask = 0x7fffffffffffffff

// TaskIDVariant is an enum with possible variants of task ID encoding.
type TaskIDVariant int

const (
	// AsRequest instructs RequestKeyToTaskID to produce an ID ending with `0`.
	AsRequest TaskIDVariant = 0
	// AsRunResult instructs RequestKeyToTaskID to produce an ID ending with `1`.
	AsRunResult TaskIDVariant = 1

	// The world started on 2010-01-01 at 00:00:00 UTC. The rationale is that using
	// EPOCH (1970) means that 40 years worth of keys are wasted.
	// 1262304000 is the result of time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	BeginningOfTheWorld int64 = 1262304000
)

// RequestKeyToTaskID converts TaskRequest entity key to a string form used in
// external APIs.
//
// For legacy reasons they are two flavors of string task IDs:
//  1. A "packed TaskRequest key", aka "packed TaskResultSummary" key. It is
//     a hex string ending with 0, e.g. `6663cfc78b41fb10`. Pass AsRequest as
//     the second argument to request this variant.
//  2. A "packed TaskRunResult key". It is a hex string ending with 1, e.g.
//     `6663cfc78b41fb11`. Pass AsRunResult as the second argument to request
//     this variant.
//
// Some APIs return the first form, others return the second. There's no clear
// logical reason why they do so anymore. They do it for backward compatibility
// with much older API, where these differences mattered.
//
// Panics if `key` is not a TaskRequest key.
func RequestKeyToTaskID(key *datastore.Key, variant TaskIDVariant) string {
	if key.Kind() != "TaskRequest" {
		panic(fmt.Sprintf("expecting TaskRequest key, but got %q", key.Kind()))
	}
	switch variant {
	case AsRequest:
		return fmt.Sprintf("%x0", key.IntID()^taskRequestIDMask)
	case AsRunResult:
		return fmt.Sprintf("%x1", key.IntID()^taskRequestIDMask)
	default:
		panic(fmt.Sprintf("invalid variant %d", variant))
	}
}

// TaskIDToRequestKey returns TaskRequest entity key given a task ID string.
//
// The task ID is something that looks like `6663cfc78b41fb10`, it is either
// a "packed TaskRequest key" (when ends with 0) or "a packed TaskRunResult key"
// (when ends with non-0). See RequestKeyToTaskID.
//
// Task request key is a root key of the hierarchy of entities representing
// a particular task. All key constructor functions for such entities take
// the request key as an argument.
func TaskIDToRequestKey(ctx context.Context, taskID string) (*datastore.Key, error) {
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

// TimestampToRequestKey converts a timestamp to a request key.
// Note that this function does NOT accept a task id. This functions is primarily
// meant for limiting queries to a task creation range.
func TimestampToRequestKey(ctx context.Context, timestamp time.Time, suffix int64) (*datastore.Key, error) {
	if suffix < 0 || suffix > 0xffff {
		return nil, errors.Reason("invalid suffix").Err()
	}
	unixts := timestamp.Unix()
	if unixts < BeginningOfTheWorld {
		return nil, errors.Reason("time %s is set to before %d", timestamp, BeginningOfTheWorld).Err()
	}
	delta := unixts - BeginningOfTheWorld
	base := (delta * 1000) << 20
	reqID := base | suffix<<4 | 0x1
	return datastore.NewKey(ctx, "TaskRequest", "", reqID^taskRequestIDMask, nil), nil
}
