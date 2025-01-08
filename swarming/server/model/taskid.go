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
//
// Xoring with it flips first 63 bits of int64 (i.e. all of them except the most
// significant bit, which is used to represent a sign in int64: better to leave
// it alone).
//
// This allows to derive datastore keys from timestamps, but order them in
// reverse chronological order (most recent first). Without xoring we'd have
// to create a special index (because keys by default are ordered in increasing
// order).
const taskRequestIDMask = 0x7fffffffffffffff

// TaskIDVariant is an enum with possible variants of task ID encoding.
type TaskIDVariant int

const (
	// AsRequest instructs RequestKeyToTaskID to produce an ID ending with `0`.
	AsRequest TaskIDVariant = 0
	// AsRunResult instructs RequestKeyToTaskID to produce an ID ending with `1`.
	AsRunResult TaskIDVariant = 1
)

var (
	// BeginningOfTheWorld is used as beginning of time when constructing request
	// keys: number of milliseconds since BeginningOfTheWorld is part of the key.
	//
	// The world started on 2010-01-01 at 00:00:00 UTC. The rationale is that
	// using the original Unix epoch (1970) results in 40 years worth of key space
	// wasted.
	//
	// We allocate 43 bits in the key for storing the timestamp at millisecond
	// precision. This makes this scheme good for 2**43 / 365 / 24 / 3600 / 1000,
	// which 278 years. We'll have until 2010+278 = 2288 before we run out of
	// key space. Should be good enough for now. Can be fixed later.
	BeginningOfTheWorld = time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
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
//
// Task id is a 64 bits integer represented as a string to the user:
//   - 1 highest order bits set to 0 to keep value positive - see
//     `taskRequestIDMask`.
//   - 43 bits is time since `BeginningOfTheWorld` at 1ms resolution - see
//     `BeginningOfTheWorld` for more details.
//   - 16 bits set to a random value or a server instance specific value.
//     Assuming an instance is internally consistent with itself, it can ensure
//     to not reuse the same 16 bits in two consecutive requests and/or throttle
//     itself to one request per millisecond.
//     Using random value reduces to 2**-15 the probability of collision on
//     exact same timestamp at 1ms resolution, so a maximum theoretical rate of
//     65536000 requests/sec but an effective rate in the range of ~64k
//     requests/sec without much transaction conflicts. We should be fine.
//   - 4 bits set to 0x1. This is to represent the 'version' of the entity
//     schema. Previous version had 0. Note that this value is XOR'ed in the DB
//     so it's stored as 0xE. When the TaskRequest entity tree is modified in a
//     breaking way that affects the packing and unpacking of task ids, this
//     value should be bumped.
//
// The key id is this value XORed with `taskRequestIDMask` - also see
// `taskRequestIDMask` for more details.
//
// Note that this function does NOT accept a task id. This functions is
// primarily meant for creating new request keys and limiting queries to a task
// creation time range.
func TimestampToRequestKey(ctx context.Context, timestamp time.Time, suffix int64) (*datastore.Key, error) {
	if suffix < 0 || suffix > 0xffff {
		return nil, errors.Reason("invalid suffix").Err()
	}
	deltaMS := timestamp.Sub(BeginningOfTheWorld).Milliseconds()
	if deltaMS < 0 {
		return nil, errors.Reason("time %s is before epoch %s", timestamp, BeginningOfTheWorld).Err()
	}
	base := deltaMS << 20
	reqID := base | suffix<<4 | 0x1
	return datastore.NewKey(ctx, "TaskRequest", "", reqID^taskRequestIDMask, nil), nil
}
