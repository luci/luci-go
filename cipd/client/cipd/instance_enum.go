// Copyright 2018 The LUCI Authors.
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

package cipd

import "context"

// InstanceEnumerator emits a list of instances, fetching them in batches.
//
// Returned by ListInstances call.
type InstanceEnumerator interface {
	// Next returns up to 'limit' instances or 0 if there's no more.
	Next(ctx context.Context, limit int) ([]InstanceInfo, error)
}

// instanceEnumeratorImpl implements InstanceEnumerator by calling the given
// callback that fetches the next page of results.
type instanceEnumeratorImpl struct {
	fetch func(ctx context.Context, limit int, cursor string) (out []InstanceInfo, nextCursor string, err error)

	cursor string // last fetched cursor or "" at the start of the fetch
	done   bool   // true if fetched the last page
}

func (e *instanceEnumeratorImpl) Next(ctx context.Context, limit int) (out []InstanceInfo, err error) {
	if e.done {
		return nil, nil
	}
	out, nextCursor, err := e.fetch(ctx, limit, e.cursor)
	if err != nil {
		return nil, err
	}
	e.cursor = nextCursor
	e.done = nextCursor == ""
	return
}
