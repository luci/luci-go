// Copyright 2023 The LUCI Authors.
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

import (
	"encoding/base64"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestEncodeAndDecode(t *testing.T) {
	ftt.Run(`Encode and decode should return the same result`, t, func(t *ftt.Test) {
		history := History{
			Runs: []Run{
				{
					CommitPosition: 1345,
					Hour:           time.Unix(1000*3600, 0),
					// Simple expected pass. Such runs represent ~97% of
					// runs in practice and are treated specially in the encoding.
					Expected: ResultCounts{PassCount: 1},
				},
				{
					CommitPosition: 1355,
					Hour:           time.Unix(1005*3600, 0),
					Expected: ResultCounts{
						PassCount:  1,
						FailCount:  2,
						CrashCount: 3,
						AbortCount: 4,
					},
					Unexpected: ResultCounts{
						PassCount:  5,
						FailCount:  6,
						CrashCount: 7,
						AbortCount: 8,
					},
				},
				{
					CommitPosition: 1355,
					Hour:           time.Unix(1005*3600, 0),
				},
				{
					CommitPosition: 1357,
					Expected:       ResultCounts{PassCount: 1},
					Hour:           time.Unix(1003*3600, 0),
				},
				{
					CommitPosition: 1357,
					Hour:           time.Unix(1005*3600, 0),
					Expected: ResultCounts{
						PassCount: 1,
					},
					Unexpected: ResultCounts{
						FailCount: 2,
					},
				},
				{
					CommitPosition: 1357,
					Hour:           time.Unix(1005*3600, 0),
					Expected: ResultCounts{
						PassCount:  9,
						FailCount:  10,
						CrashCount: 11,
						AbortCount: 12,
					},
					Unexpected: ResultCounts{
						PassCount:  13,
						FailCount:  14,
						CrashCount: 15,
						AbortCount: 16,
					},
				},
			},
		}

		hs := &HistorySerializer{}
		encoded := hs.Encode(history)
		decodedHistory := History{
			Runs: make([]Run, 0, 100),
		}
		err := hs.DecodeInto(&decodedHistory, encoded)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(decodedHistory.Runs), should.Equal(6))
		assert.Loosely(t, decodedHistory, should.Match(history))
	})

	ftt.Run(`Encode and decode long history should not have error`, t, func(t *ftt.Test) {
		history := History{}
		history.Runs = make([]Run, 2000)
		for i := 0; i < 2000; i++ {
			history.Runs[i] = Run{
				CommitPosition: int64(i),
				Hour:           time.Unix(int64(i*3600), 0),
				Expected: ResultCounts{
					PassCount: 1,
				},
				Unexpected: ResultCounts{
					FailCount: 2,
				},
			}
		}
		hs := &HistorySerializer{}
		encoded := hs.Encode(history)
		decodedHistory := History{
			Runs: make([]Run, 0, 2000),
		}
		err := hs.DecodeInto(&decodedHistory, encoded)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(decodedHistory.Runs), should.Equal(2000))
		assert.Loosely(t, decodedHistory, should.Match(history))
	})
	ftt.Run(`Decode legacy data format v2`, t, func(t *ftt.Test) {
		t.Run(`Short history`, func(t *ftt.Test) {
			// Four verdicts.
			b, err := base64.StdEncoding.DecodeString("enRkCii1L/0EAKEBAAIEghXQDxUKAAIBAgMEBQYHCAABAAAAAAIAAAEEAwEEAQIBAAAAAAIAAAEJCgsMDQ4PEACx/UmM")
			assert.Loosely(t, err, should.BeNil)

			decodedHistory := History{
				Runs: make([]Run, 0, 100),
			}
			hs := &HistorySerializer{}
			err = hs.DecodeInto(&decodedHistory, b)
			assert.Loosely(t, err, should.BeNil)

			expectedRuns := []Run{
				{
					CommitPosition: 1345,
					Hour:           time.Unix(1000*3600, 0),
					Expected:       ResultCounts{PassCount: 1},
				},
				{
					CommitPosition: 1355,
					Hour:           time.Unix(1005*3600, 0),
					Expected: ResultCounts{
						PassCount:  1,
						FailCount:  2,
						CrashCount: 3,
						AbortCount: 4,
					},
					Unexpected: ResultCounts{
						PassCount:  5,
						FailCount:  6,
						CrashCount: 7,
						AbortCount: 8,
					},
				},
				{
					CommitPosition: 1357,
					Expected:       ResultCounts{PassCount: 1},
					Hour:           time.Unix(1003*3600, 0),
				},
				{
					CommitPosition: 1357,
					Hour:           time.Unix(1005*3600, 0),
					Expected: ResultCounts{
						PassCount:  9,
						FailCount:  10,
						CrashCount: 11,
						AbortCount: 12,
					},
					Unexpected: ResultCounts{
						PassCount:  13,
						FailCount:  14,
						CrashCount: 15,
						AbortCount: 16,
					},
				},
			}
			assert.Loosely(t, decodedHistory.Runs, should.Match(expectedRuns))
		})
		t.Run(`Long history`, func(t *ftt.Test) {
			var expectedRuns []Run
			for i := 0; i < 2000; i++ {
				for j := 0; j < 3; j++ {
					expectedRuns = append(expectedRuns, Run{
						CommitPosition: int64(i),
						Hour:           time.Unix(int64(i*3600), 0),
						Expected:       ResultCounts{PassCount: 1},
						Unexpected:     ResultCounts{FailCount: 2},
					})
				}
			}

			// 2000 verdicts, of 3 runs each.
			// The string is so small because they all have incrementing
			// hours and positions, which means they compresses extremely well in
			// the encoding.
			b, err := base64.StdEncoding.DecodeString("enRkCii1L/1kM/HtAAAiAQQK4EEA//+/3fDe99r12YFQAgAMcqF0jF5ZAnoIHDw=")
			assert.Loosely(t, err, should.BeNil)

			decodedHistory := History{
				Runs: make([]Run, 0, 100),
			}
			hs := &HistorySerializer{}
			err = hs.DecodeInto(&decodedHistory, b)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, decodedHistory.Runs, should.Match(expectedRuns))
		})
	})
}

func TestInputBuffer(t *testing.T) {
	ftt.Run(`Add item to input buffer`, t, func(t *ftt.Test) {
		ib := NewWithCapacity(10, 100)
		originalHotBuffer := ib.HotBuffer.Runs
		originalColdBuffer := ib.ColdBuffer.Runs

		// Insert 9 verdicts into hot buffer.
		ib.InsertRun(createTestRun(1, 4))
		ib.InsertRun(createTestRun(2, 2))
		ib.InsertRun(createTestRun(3, 3))
		ib.InsertRun(createTestRun(2, 3))
		ib.InsertRun(createTestRun(4, 5))
		ib.InsertRun(createTestRun(1, 1))
		ib.InsertRun(createTestRun(2, 3))
		ib.InsertRun(createTestRun(7, 8))
		ib.InsertRun(createTestRun(7, 7))

		assert.Loosely(t, ib.IsColdBufferDirty, should.BeFalse)

		// Expect compaction to have no effect.
		ib.CompactIfRequired()
		assert.Loosely(t, ib.IsColdBufferDirty, should.BeFalse)
		assert.Loosely(t, len(ib.HotBuffer.Runs), should.Equal(9))
		assert.Loosely(t, ib.HotBuffer.Runs, should.Match([]Run{
			createTestRun(1, 1),
			createTestRun(1, 4),
			createTestRun(2, 2),
			createTestRun(2, 3),
			createTestRun(2, 3),
			createTestRun(3, 3),
			createTestRun(4, 5),
			createTestRun(7, 7),
			createTestRun(7, 8),
		}))

		// Insert the last verdict.
		ib.InsertRun(createTestRun(6, 2))
		assert.Loosely(t, ib.IsColdBufferDirty, should.BeFalse)

		// Compaction should not have an effect.
		ib.CompactIfRequired()
		assert.Loosely(t, ib.IsColdBufferDirty, should.BeTrue)
		assert.Loosely(t, len(ib.HotBuffer.Runs), should.BeZero)
		assert.Loosely(t, len(ib.ColdBuffer.Runs), should.Equal(10))
		assert.Loosely(t, ib.ColdBuffer.Runs, should.Match([]Run{
			createTestRun(1, 1),
			createTestRun(1, 4),
			createTestRun(2, 2),
			createTestRun(2, 3),
			createTestRun(2, 3),
			createTestRun(3, 3),
			createTestRun(4, 5),
			createTestRun(6, 2),
			createTestRun(7, 7),
			createTestRun(7, 8),
		}))

		// The pre-allocated buffer should be retained, at the same capacity.
		assert.Loosely(t, &ib.HotBuffer.Runs[0:1][0], should.Equal(&originalHotBuffer[0:1][0]))
		assert.Loosely(t, &ib.ColdBuffer.Runs[0], should.Equal(&originalColdBuffer[0:1][0]))
	})
	ftt.Run(`Test result flow is efficient`, t, func(t *ftt.Test) {
		ib := NewWithCapacity(10, 100)
		coldBufferDirtyCount := 0
		evictionCount := 0
		for i := 0; i < 1000; i++ {
			ib.InsertRun(createTestRun(i, i))
			ib.CompactIfRequired()

			shouldEvict, endIndex := ib.EvictionRange()
			if shouldEvict {
				ib.ColdBuffer.EvictBefore(endIndex + 1)
				evictionCount++
			}

			if ib.IsColdBufferDirty {
				coldBufferDirtyCount++
				// Pretend we flushed out the buffer to Spanner.
				ib.IsColdBufferDirty = false
			}
		}
		// We should not have to write the cold buffer out to Spanner
		// every time, but only once per HotBufferCapacity times.
		assert.Loosely(t, coldBufferDirtyCount, should.BeLessThanOrEqual(100))
		// Eviction is similar, but only starts once the cold buffer
		// is full.
		assert.Loosely(t, evictionCount, should.Equal(90))
	})
	ftt.Run(`Compaction should maintain order`, t, func(t *ftt.Test) {
		ib := Buffer{
			HotBufferCapacity: 5,
			HotBuffer: History{
				Runs: []Run{
					createTestRun(1, 1),
					createTestRun(3, 1),
					createTestRun(5, 1),
					createTestRun(7, 1),
					createTestRun(9, 1),
				},
			},
			ColdBufferCapacity: 10,
			ColdBuffer: History{
				// Allocate with capacity 10 so there is enough
				// space to do an in-place compaction.
				Runs: append(make([]Run, 0, 10), []Run{
					createTestRun(2, 1),
					createTestRun(4, 1),
					createTestRun(6, 1),
					createTestRun(8, 1),
					createTestRun(10, 1),
				}...),
			},
		}
		originalHotBuffer := ib.HotBuffer.Runs
		originalColdBuffer := ib.ColdBuffer.Runs
		assert.Loosely(t, cap(originalColdBuffer), should.Equal(10))

		ib.CompactIfRequired()
		assert.Loosely(t, len(ib.HotBuffer.Runs), should.BeZero)
		assert.Loosely(t, len(ib.ColdBuffer.Runs), should.Equal(10))
		assert.Loosely(t, ib.ColdBuffer.Runs, should.Match([]Run{
			createTestRun(1, 1),
			createTestRun(2, 1),
			createTestRun(3, 1),
			createTestRun(4, 1),
			createTestRun(5, 1),
			createTestRun(6, 1),
			createTestRun(7, 1),
			createTestRun(8, 1),
			createTestRun(9, 1),
			createTestRun(10, 1),
		}))

		// The pre-allocated buffer should be retained, at the same capacity.
		assert.Loosely(t, &ib.HotBuffer.Runs[0:1][0], should.Equal(&originalHotBuffer[0:1][0]))
		assert.Loosely(t, &ib.ColdBuffer.Runs[0], should.Equal(&originalColdBuffer[0:1][0]))
	})

	ftt.Run(`Cold buffer should keep old verdicts after compaction`, t, func(t *ftt.Test) {
		ib := Buffer{
			HotBufferCapacity: 2,
			HotBuffer: History{
				Runs: []Run{
					createTestRun(7, 1),
					createTestRun(9, 1),
				},
			},
			ColdBufferCapacity: 5,
			ColdBuffer: History{
				Runs: []Run{
					createTestRun(2, 1),
					createTestRun(4, 1),
					createTestRun(6, 1),
					createTestRun(8, 1),
					createTestRun(10, 1),
				},
			},
		}

		ib.CompactIfRequired()
		assert.Loosely(t, len(ib.HotBuffer.Runs), should.BeZero)
		assert.Loosely(t, len(ib.ColdBuffer.Runs), should.Equal(7))
		assert.Loosely(t, ib.ColdBuffer.Runs, should.Match([]Run{
			createTestRun(2, 1),
			createTestRun(4, 1),
			createTestRun(6, 1),
			createTestRun(7, 1),
			createTestRun(8, 1),
			createTestRun(9, 1),
			createTestRun(10, 1),
		}))
	})

	ftt.Run(`EvictBefore`, t, func(t *ftt.Test) {
		buffer := History{
			Runs: []Run{
				createTestRun(2, 1),
				createTestRun(4, 1),
				createTestRun(6, 1),
				createTestRun(8, 1),
				createTestRun(10, 1),
			},
		}
		originalVerdictsBuffer := buffer.Runs
		assert.Loosely(t, cap(buffer.Runs), should.Equal(5))

		t.Run(`Start of slice`, func(t *ftt.Test) {
			buffer.EvictBefore(0)
			assert.Loosely(t, buffer, should.Match(History{
				Runs: []Run{
					createTestRun(2, 1),
					createTestRun(4, 1),
					createTestRun(6, 1),
					createTestRun(8, 1),
					createTestRun(10, 1),
				},
			}))

			assert.Loosely(t, &buffer.Runs[0:1][0], should.Equal(&originalVerdictsBuffer[0]))
			assert.Loosely(t, cap(buffer.Runs), should.Equal(5))
		})
		t.Run(`Middle of slice`, func(t *ftt.Test) {
			buffer.EvictBefore(2)
			assert.Loosely(t, buffer, should.Match(History{
				Runs: []Run{
					createTestRun(6, 1),
					createTestRun(8, 1),
					createTestRun(10, 1),
				},
			}))

			// The pre-allocated buffer should be retained, at the same capacity.
			assert.Loosely(t, &buffer.Runs[0:1][0], should.Equal(&originalVerdictsBuffer[0]))
			assert.Loosely(t, cap(buffer.Runs), should.Equal(5))
		})
		t.Run(`End of slice`, func(t *ftt.Test) {
			buffer.EvictBefore(5)
			assert.Loosely(t, buffer, should.Match(History{
				Runs: []Run{},
			}))

			// The pre-allocated buffer should be retained, at the same capacity.
			assert.Loosely(t, &buffer.Runs[0:1][0], should.Equal(&originalVerdictsBuffer[0]))
			assert.Loosely(t, cap(buffer.Runs), should.Equal(5))
		})
		t.Run(`Empty slice`, func(t *ftt.Test) {
			buffer := History{
				Runs: []Run{},
			}
			buffer.EvictBefore(0)
			assert.Loosely(t, buffer, should.Match(History{
				Runs: []Run{},
			}))
		})
	})

	ftt.Run(`Clear`, t, func(t *ftt.Test) {
		ib := NewWithCapacity(5, 10)
		ib.HotBuffer.Runs = append(ib.HotBuffer.Runs, []Run{
			createTestRun(1, 1),
			createTestRun(3, 1),
			createTestRun(5, 1),
			createTestRun(7, 1),
			createTestRun(9, 1),
		}...)
		ib.ColdBuffer.Runs = append(ib.ColdBuffer.Runs, []Run{
			createTestRun(2, 1),
			createTestRun(4, 1),
			createTestRun(6, 1),
			createTestRun(8, 1),
			createTestRun(10, 1),
		}...)
		originalHotBuffer := ib.HotBuffer.Runs
		originalColdBuffer := ib.ColdBuffer.Runs

		ib.Clear()

		assert.Loosely(t, ib, should.Match(&Buffer{
			HotBufferCapacity: 5,
			HotBuffer: History{
				Runs: []Run{},
			},
			ColdBufferCapacity: 10,
			ColdBuffer: History{
				Runs: []Run{},
			},
		}))
		// The pre-allocated buffer should be retained, at the same capacity.
		assert.Loosely(t, &ib.HotBuffer.Runs[0:1][0], should.Equal(&originalHotBuffer[0]))
		assert.Loosely(t, &ib.ColdBuffer.Runs[0:1][0], should.Equal(&originalColdBuffer[0]))
	})
}

func createTestRun(pos int, hour int) Run {
	return Run{
		CommitPosition: int64(pos),
		Hour:           time.Unix(int64(3600*hour), 0),
		Expected: ResultCounts{
			PassCount: 1,
		},
	}
}

func BenchmarkEncode(b *testing.B) {
	// Last known result (Jun-2024):
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkEncode-96    	   26706	     46593 ns/op	    2160 B/op	       2 allocs/op

	b.StopTimer()
	hs := &HistorySerializer{}
	hs.ensureAndClearBuf()
	ib := &Buffer{
		HotBufferCapacity:  100,
		HotBuffer:          History{Runs: simpleVerdicts(100, 102_000, []int{5})},
		ColdBufferCapacity: 2000,
		ColdBuffer:         History{Runs: simpleVerdicts(2000, 100_000, []int{102, 174, 872, 971})},
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		hs.Encode(ib.ColdBuffer)
		hs.Encode(ib.HotBuffer)
	}
}

func BenchmarkDecode(b *testing.B) {
	// Last known result (Jun-2024):
	// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
	// BenchmarkDecode-96    	   22759	     52699 ns/op	      96 B/op	       2 allocs/op

	b.StopTimer()
	var hs HistorySerializer
	hs.ensureAndClearBuf()
	inputBuffer := NewWithCapacity(100, 2000)

	ib := &Buffer{
		HotBufferCapacity:  100,
		HotBuffer:          History{Runs: simpleVerdicts(100, 102_000, []int{5})},
		ColdBufferCapacity: 2000,
		ColdBuffer:         History{Runs: simpleVerdicts(2000, 100_000, []int{102, 174, 872, 971})},
	}
	encodedColdBuffer := hs.Encode(ib.ColdBuffer) // 66 bytes compressed
	encodedHotBuffer := hs.Encode(ib.HotBuffer)   // 42 bytes compressed
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := hs.DecodeInto(&inputBuffer.ColdBuffer, encodedColdBuffer)
		if err != nil {
			panic(err)
		}
		err = hs.DecodeInto(&inputBuffer.HotBuffer, encodedHotBuffer)
		if err != nil {
			panic(err)
		}
	}
}
