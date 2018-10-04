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

// Package transient allows you to tag and retry 'transient' errors (i.e.
// non-permanent errors which may resolve themselves by trying an operation
// again). This should be used on errors due to network flake, improperly
// responsive remote servers (e.g. status 500), unusual timeouts, etc. where
// there's no concrete knowledge that something is permanently wrong.
//
// Said another way, transient errors appear to resolve themselves with nothing
// other than the passage of time.
package transient

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
)

// transientOnlyIterator is an Iterator implementation that only retries errors
// if they are tagged with `transient.Tag`.
type transientOnlyIterator struct {
	retry.Iterator // The wrapped Iterator.
}

func (i *transientOnlyIterator) Next(ctx context.Context, err error) time.Duration {
	if !Tag.In(err) {
		return retry.Stop
	}
	return i.Iterator.Next(ctx, err)
}

// Only returns a retry.Iterator that wraps another retry.Iterator. It
// will fall through to the wrapped Iterator ONLY if an error with the
// transient.Tag is encountered; otherwise, it will not retry.
//
// Returns nil if f is nil.
//
// Example:
//   err := retry.Retry(c, transient.Only(retry.Default), func() error {
//     if condition == "red" {
//		   // This error isn't transient, so it won't be retried.
//		   return errors.New("fatal bad condition")
//     } elif condition == "green" {
//		   // This isn't an error, so it won't be retried.
//		   return nil
//     }
//     // This will get retried, because it's transient.
//     return errors.New("dunno what's wrong", transient.Tag)
//   })
func Only(next retry.Factory) retry.Factory {
	if next == nil {
		return nil
	}
	return func() retry.Iterator {
		if it := next(); it != nil {
			return &transientOnlyIterator{it}
		}
		return nil
	}
}

// Tag is used to indicate that an error is transient (i.e. something is
// temporarially wrong).
var Tag = errors.BoolTag{Key: errors.NewTagKey("this error is temporary")}
