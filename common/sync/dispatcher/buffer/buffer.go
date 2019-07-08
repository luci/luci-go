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

package buffer

import (
	"context"
	"time"
)

// Buffer batches individual data items into Batch objects
type Buffer interface {
	Add(item interface{}) bool

	NextSendTime() time.Time
	IsFull() bool

	Pop() *Batch
	ReQueue(*Batch)
}

type bufferImpl struct {
	ctx  context.Context
	opts Options

	heap        batchHeap
	lastBatchID uint64
}

var _ Buffer = &bufferImpl{}

func NewBuffer(ctx context.Context, o Options) Buffer {
	return &bufferImpl{ctx: ctx, opts: o}
}

func (b *bufferImpl) Add(item interface{}) bool { return false }
func (b *bufferImpl) NextSendTime() time.Time   { return time.Time{} }
func (b *bufferImpl) IsFull() bool              { return true }
func (b *bufferImpl) Pop() *Batch               { return nil }
func (b *bufferImpl) ReQueue(*Batch)            {}
