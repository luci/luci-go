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

import "context"

// MockClient is for testing purposes.
type MockClient struct {
	// SendRowFn is called on `SendRows`.
	FetchLatestFn func(context.Context, string) (Status, error)
	SentRows      []proto.Message
}

var _ Client = (*MockClient)(nil)

// SendRow sends the row(s) to a BQ table.
func (m *MockClient) SendRows(ctx context.Context, dataset, table string) (Status, error) {
	return m.SendRowFn(ctx, dataset, table)
}
