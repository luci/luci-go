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
// connected to a (potentially) parallelized work dispatcher.
//
// This can be used when you have a mismatch between the rate of production of
// items and the rate of consumption of those items. For example:
//
//   - if you have a producer which constantly produces new world states,
//     and you want to sink the latest one into a slow external RPC (but still
//     do retries if no new state appears).
//   - if you have bursty user data which you'd like to batch according to some
//     maximum batch size, but you don't want the data to get too stale in case
//     you don't hit that batch limit.
//   - your external RPC can absorb almost infinite data, and the order of
//     delivery doesn't matter, but you don't want to block the data producer.
//   - etc.
//
// The dispatcher can be configured to:
//   - Buffer a certain amount of work (with possible backpressure to the
//     producer).
//   - Batch pending work into chunks for the send function.
//   - Drop stale work which is no longer important to send.
//   - Enforce a maximum QPS on the send function (even with parallel senders).
//   - Retry batches independently with configurable per-batch retry policy.
package dispatcher
