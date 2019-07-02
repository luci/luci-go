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

// SendBuffer represents an ordered Queue of Batches available to Send.
type SendBuffer struct{}

// Add causes the provided Batch to be immediately available to send. If this
// adds a Batch already in the SendBuffer, it resets the Batch's internal wait
// timer to Options.MinSleep.
func (*SendBuffer) Add(*Batch) {}

// Remove causes the provided Batch to be dropped from the queue.
func (*SendBuffer) Remove(*Batch) {}

// Len returns the number of Batches in this SendBuffer.
func (*SendBuffer) Len() int { panic("not implemented") }

// Clear empties the queue entirely.
func (*SendBuffer) Clear() {}

// Batch represents a collection of individual work items and associated
// metdata.
//
// Batches are opaque to Channel; they are created by NewItemFn, and can be
// manipulated by ErrorFn and SendFn. This may be useful to do things such as:
//   * Associate a UID with the Batch (e.g. in the Meta field) to identify it to
//     remote services for deduplication.
//   * Remove already-processed items from Data in case the SendFn partially
//     succeeded.
type Batch struct {
	// Data is the actual items in the batch.
	Data []interface{}

	Meta interface{}
}
