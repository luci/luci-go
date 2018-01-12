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

package bq

import (
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"
	"time"
)

var (
	// defaultPrefix is a global value is used to populate a zero-value
	// InsertIDGenerator.
	defaultPrefix string
)

func init() {
	t := time.Now().UnixNano()
	defaultPrefix = fmt.Sprintf("%d:%d:%d", rand.Int(), os.Getpid(), t)
}

// InsertIDGenerator generates unique Insert IDs.
//
// BigQuery uses Insert IDs to deduplicate rows in the streaming insert buffer.
// The association between Insert ID and row persists only for the time the row
// is in the buffer.
//
// InsertIDGenerator is safe for concurrent use.
type InsertIDGenerator struct {
	// Counter is an atomically-managed counter used to differentiate Insert
	// IDs produced by the same process.
	Counter int64
	// Prefix should be able to uniquely identify this specific process,
	// to differentiate Insert IDs produced by different processes.
	//
	// If empty, prefix will be derived from system and process specific
	// properties.
	Prefix string
}

// Generate returns a unique Insert ID.
func (id *InsertIDGenerator) Generate() string {
	prefix := id.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}
	c := atomic.AddInt64(&id.Counter, 1)
	return fmt.Sprintf("%s:%d", prefix, c)
}
