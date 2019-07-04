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

package dispatcher

import (
	"go.chromium.org/luci/common/errors"
)

// Normalize validates that Options is well formed and populates defaults which
// are missing.
func (o *Options) Normalize() error {
	if o.SendFn == nil {
		return errors.New("required field `SendFn` == nil")
	}

	if o.ErrorFn == nil {
		o.ErrorFn = defaultErrorFn
	}

	if err := o.Concurrency.Normalize(); err != nil {
		return errors.Annotate(err, "normalizing Concurrency").Err()
	}

	if err := o.Retry.Normalize(); err != nil {
		return errors.Annotate(err, "normalizing Retry").Err()
	}

	if err := o.Batch.Normalize(); err != nil {
		return errors.Annotate(err, "normalizing Batch").Err()
	}

	if err := o.Buffer.Normalize(); err != nil {
		return errors.Annotate(err, "normalizing Buffer").Err()
	}

	return nil
}

// Normalize validates that ConcurrencyOptions is well formed and populates
// defaults which are missing.
func (o *ConcurrencyOptions) Normalize() error {
	def := defaultConcurrencyOptions

	if o.MaxSenders == 0 {
		o.MaxSenders = def.MaxSenders
	} else if o.MaxSenders < 1 {
		return errors.Reason("MaxSenders must be > 0: got %d", o.MaxSenders).Err()
	}

	if o.MaxQPS == 0 {
		o.MaxQPS = def.MaxQPS
	} else if o.MaxQPS < 0 {
		return errors.Reason("MaxQPS must be > 0: got %f", o.MaxQPS).Err()
	}
	return nil
}

// Normalize validates that RetryOptions is well formed and populates defaults
// which are missing.
func (o *RetryOptions) Normalize() error {
	def := defaultRetryOptions

	if o.InitialSleep == 0 {
		o.InitialSleep = def.InitialSleep
	} else if o.InitialSleep < 0 {
		return errors.Reason("InitialSleep must be > 0: got %s", o.InitialSleep).Err()
	}

	if o.MaxSleep == 0 {
		o.MaxSleep = def.MaxSleep
	} else if o.MaxSleep < o.InitialSleep {
		return errors.Reason(
			"MaxSleep must be >= InitialSleep: got MaxSleep(%s) < InitialSleep(%s)",
			o.MaxSleep, o.InitialSleep).Err()
	}

	if o.BackoffFactor == 0 {
		o.BackoffFactor = def.BackoffFactor
	} else if o.BackoffFactor <= 1 {
		return errors.Reason("BackoffFactor must be > 1.0: got %f", o.BackoffFactor).Err()
	}

	if o.Limit == 0 {
		o.Limit = def.Limit
	} else if o.Limit < 0 {
		return errors.Reason("Limit must be > 0: got %d", o.Limit).Err()
	}
	return nil
}

// Normalize validates that BatchOptions is well formed and populates defaults
// which are missing.
func (o *BatchOptions) Normalize() error {
	def := defaultBatchOptions

	switch {
	case o.MaxSize == 0:
		o.MaxSize = def.MaxSize
	case o.MaxSize > 0 || o.MaxSize == -1:
	default:
		return errors.Reason("MaxSize must be > 0 or == -1: got %d", o.MaxSize).Err()
	}

	if o.MaxDuration == 0 {
		o.MaxDuration = def.MaxDuration
	} else if o.MaxDuration < 0 {
		return errors.Reason("MaxDuration must be > 0: got %s", o.MaxDuration).Err()
	}

	return nil
}

// Normalize validates that BufferOptions is well formed and populates defaults
// which are missing.
func (o *BufferOptions) Normalize() error {
	def := defaultBufferOptions

	if o.MaxSize == 0 {
		o.MaxSize = def.MaxSize
	} else if o.MaxSize < 0 {
		return errors.Reason("MaxSize must be > 0: got %d", o.MaxSize).Err()
	}

	switch o.BufferFullBehavior {
	case 0:
		o.BufferFullBehavior = BlockNewData
	case BlockNewData, DropOldestBatch:
		// OK
	default:
		return errors.Reason("BufferFullBehavior is unknown: %s", o.BufferFullBehavior).Err()
	}

	return nil
}
