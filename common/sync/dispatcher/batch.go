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

// Batch represents a collection of individual work items and associated
// metadata.
//
// Batches are are cut by the Channel according to BatchOptions, and can be
// manipulated by ErrorFn and SendFn.
//
// ErrorFn and SendFn may manipulate the contents of the Batch (Data and Meta)
// to do things such as:
//   * Associate a UID with the Batch (e.g. in the Meta field) to identify it to
//     remote services for deduplication.
//   * Remove already-processed items from Data in case the SendFn partially
//     succeeded.
//
// The dispatcher uses Batch.Len() to account for how many items are currently
// buffered. The length of a Batch is `min(len(Data), Batch's original length)`.
type Batch struct {
	// Data is the actual items in the batch.
	Data []interface{}

	// Meta is an object which dispatcher.Channel will treat as totally opaque;
	// You may manipulate it in SendFn or ErrorFn as you see fit. This can be used
	// for e.g. associating a nonce with the Batch for retries.
	Meta interface{}

	batchID        uint64
	originalLength int
}

// Len returns the current effective length of this Batch. It's the smaller of:
//   * len(Data)
//   * the original length of this Batch when it was cut.
//
// This means that if you add items to Data, the dispatcher will account for
// this Batch as having it's original number of data items.
func (b *Batch) Len() int {
	cur := len(b.Data)
	if cur < b.originalLength {
		return cur
	}
	return b.originalLength
}
