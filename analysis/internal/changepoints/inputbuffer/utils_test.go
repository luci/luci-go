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

package inputbuffer

import "testing"

// Output as of June 2024 on Intel Skylake CPU @ 2.00GHz:
// BenchmarkMergeOrderedRuns-96    	  289780	      4080 ns/op	       2 B/op	       0 allocs/op
func BenchmarkMergeOrderedRuns(b *testing.B) {
	b.StopTimer()
	var hotBuffer, coldBuffer []Run

	// Consistently passing test. This represents ~99% of tests.
	for i := range 100 {
		hotBuffer = append(hotBuffer, Run{
			CommitPosition: int64(i),
			Expected:       ResultCounts{PassCount: 1},
		})
	}

	for i := range 2000 {
		coldBuffer = append(coldBuffer, Run{
			CommitPosition: int64(i),
			Expected:       ResultCounts{PassCount: 1},
		})
	}

	dest := make([]*Run, 0, 2100)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		MergeOrderedRuns(hotBuffer, coldBuffer, &dest)
	}
}
