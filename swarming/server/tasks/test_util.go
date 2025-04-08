// Copyright 2024 The LUCI Authors.
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

package tasks

import (
	"context"

	"go.chromium.org/luci/gae/service/datastore"
)

// MockedManager is used exclusively in tests.
//
// It just forward the calls to the corresponding callbacks that are expected
// to be populated by tests.
type MockedManager struct {
	CreateTaskMock         func(context.Context, *CreationOp) (*CreatedTask, error)
	EnqueueBatchCancelMock func(context.Context, []string, bool, string, int32) error
	ClaimTxnMock           func(context.Context, *ClaimOp) (*ClaimOpOutcome, error)
	AbandonTxnMock         func(context.Context, *AbandonOp) (*AbandonOpOutcome, error)
	CancelTxnMock          func(context.Context, *CancelOp) (*CancelOpOutcome, error)
	CompleteTxnMock        func(context.Context, *CompleteOp) (*CompleteTxnOutcome, error)
}

func (m *MockedManager) CreateTask(ctx context.Context, op *CreationOp) (*CreatedTask, error) {
	if datastore.CurrentTransaction(ctx) != nil {
		panic("must not be in a transaction")
	}
	return m.CreateTaskMock(ctx, op)
}

func (m *MockedManager) EnqueueBatchCancel(ctx context.Context, batch []string, killRunning bool, purpose string, retries int32) error {
	if datastore.CurrentTransaction(ctx) != nil {
		panic("must not be in a transaction")
	}
	return m.EnqueueBatchCancelMock(ctx, batch, killRunning, purpose, retries)
}

func (m *MockedManager) ClaimTxn(ctx context.Context, op *ClaimOp) (*ClaimOpOutcome, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be in a transaction")
	}
	return m.ClaimTxnMock(ctx, op)
}

func (m *MockedManager) AbandonTxn(ctx context.Context, op *AbandonOp) (*AbandonOpOutcome, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be in a transaction")
	}
	return m.AbandonTxnMock(ctx, op)
}

func (m *MockedManager) CancelTxn(ctx context.Context, op *CancelOp) (*CancelOpOutcome, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be in a transaction")
	}
	return m.CancelTxnMock(ctx, op)
}

func (m *MockedManager) CompleteTxn(ctx context.Context, op *CompleteOp) (*CompleteTxnOutcome, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be in a transaction")
	}
	return m.CompleteTxnMock(ctx, op)
}
