// Copyright 2025 The LUCI Authors.
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

package delta

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// Collect allocates a new `T`, and applies all `diffs` to it, or returns an
// error if any of the `diffs` contained an error.
func Collect[T proto.Message](diffs ...*Diff[T]) (T, error) {
	var zero T
	ret := zero.ProtoReflect().New().Interface().(T)
	return ret, CollectInto(ret, diffs...)
}

// CollectInto applies all `diffs` to the provided message `T`, or returns an
// error if any of the `diffs` contained an error.
func CollectInto[T proto.Message](msg T, diffs ...*Diff[T]) error {
	var errs []error
	for i, diff := range diffs {
		if diff.err != nil {
			errs = append(errs, fmt.Errorf("diff[%d]: %w", i, diff.err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	for _, diff := range diffs {
		diff.Apply(msg)
	}
	return nil
}
