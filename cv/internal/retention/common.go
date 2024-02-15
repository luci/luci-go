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

package retention

import (
	"errors"
	"time"
)

var retentionPeriod = 540 * 24 * time.Hour // ~= 1.5 years
// wipeoutTasksDistInterval defines the interval that wipeout tasks will be
// evenly distributed.
var wipeoutTasksDistInterval = 1 * time.Hour

// chunk splits []T into chunks of provided size.
//
// If the slice cannot be split evenly, the last chunk will contain all the
// remaining elements. The provided size must be larger than 0.
func chunk[T any](slice []T, size int) [][]T {
	if size <= 0 {
		panic(errors.New("size must be larger than 0"))
	}
	var chunks [][]T
	for i := 0; i < len(slice); i += size {
		end := min(i+size, len(slice))
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}
