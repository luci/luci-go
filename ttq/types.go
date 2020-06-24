// Copyright 2020 The LUCI Authors.
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

package ttq

import (
	"context"
)

// PostProcess should be executed after the transaction completes to speed up
// the enqueueing process.
// TODO(tandrii): document.
type PostProcess func(context.Context)

// Options configures operations of shared parts of the TTQ library.
//
// Only the Queue name must be specified. There are defaults for all others
// which should work for most apps with <5000 QPS of task enqueues.
// TODO(tandrii): document.
type Options struct {
	Queue string
	// TODO(tandrii): implement.
}
