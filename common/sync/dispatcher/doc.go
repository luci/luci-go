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

// Package dispatcher implements a super-charged version of a buffered channel
// connected to a parallelized work dispatcher.
//
// This can be used when you have a mismatch between the rate of production of
// items and the rate of consumption of those items. For example, if you have
// a producer which constantly produces new world states, and you want to sink
// them into a slow external RPC.
//
// The dispatcher can be configured to:
//   * Buffer a certain amount of work (with possible backpressure to the
//     producer).
//   * Batch pending work into more chunks for the send function.
//   * Drop stale work which is no longer important to send.
//   * Wait a minimum amount of time between calling the send function.
//   * Do per-batch exponential backoff.
package dispatcher
