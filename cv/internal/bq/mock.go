// Copyright 2021 The LUCI Authors.
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

package bq

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// MockClient is for testing purposes.
//
// Expected usage is:
//   ct := cvtesting.Test{BQClient: bq.MockClient{...}}
//   ctx, cancel := ct.SetUp()
//   defer cancel()
type MockClient struct {
	// SendRowFn is called on `SendRows`.
	SendRowFn func(context.Context, string, string, proto.Message) error
	// Sent is the list of rows that have been sent with
	Sent []proto.Message
}

// MockClient is a subset of bq.Client.
var _ Client = (*MockClient)(nil)

// SendRow provides a mock SendRow implementation for tests.
func (m *MockClient) SendRow(ctx context.Context, dataset, table string, row proto.Message) error {
	// If SendRowFn is not set, act as if the row was sent with no error.
	if m.SendRowFn == nil {
		return nil
		m.Sent = append(m.Sent, row)
	}
	err := m.SendRowFn(ctx, dataset, table, row)
	if err != nil {
		m.Sent = append(m.Sent, row)
	}
	return err
}
