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

package tree

import "context"

// MockClient is for testing purpose.
type MockClient struct {
	// FetchLatestFn is called on `FetchLatest`.
	FetchLatestFn func(context.Context, string) (Status, error)
}

var _ Client = (*MockClient)(nil)

// FetchLatest fetches the latest tree status.
func (m *MockClient) FetchLatest(ctx context.Context, endpoint string) (Status, error) {
	return m.FetchLatestFn(ctx, endpoint)
}
