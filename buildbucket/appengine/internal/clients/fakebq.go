// Copyright 2022 The LUCI Authors.
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

package clients

import (
	"context"
	"sync"

	lucibq "go.chromium.org/luci/common/bq"
)

type FakeBqClient struct {
	mu sync.RWMutex
	// data persists all inserted rows.
	data map[string][]*lucibq.Row
}

var FakeBqErrCtxKey = "used only in tests to make BQ return a fake error"

func (f *FakeBqClient) Insert(ctx context.Context, dataset, table string, row *lucibq.Row) error {
	if err, ok := ctx.Value(&FakeBqErrCtxKey).(error); ok {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := dataset + "." + table
	if f.data == nil {
		f.data = make(map[string][]*lucibq.Row)
	}
	f.data[key] = append(f.data[key], row)
	return nil
}

func (f *FakeBqClient) GetRows(dataset, table string) []*lucibq.Row{
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.data[dataset + "." + table]
}
