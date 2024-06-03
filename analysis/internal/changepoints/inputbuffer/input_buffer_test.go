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

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeAndDecode(t *testing.T) {
	Convey(`Encode and decode should return the same result`, t, func() {
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
		So(err, ShouldBeNil)
		So(len(decodedHistory.Runs), ShouldEqual, 6)
		So(decodedHistory, ShouldResemble, history)
	})

	Convey(`Encode and decode long history should not have error`, t, func() {
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
		So(err, ShouldBeNil)
		So(len(decodedHistory.Runs), ShouldEqual, 2000)
		So(decodedHistory, ShouldResemble, history)
	})
	Convey(`Decode legacy data format v2`, t, func() {
		Convey(`Short history`, func() {
			// Four verdicts.
			b, err := base64.StdEncoding.DecodeString("enRkCii1L/0EAKEBAAIEghXQDxUKAAIBAgMEBQYHCAABAAAAAAIAAAEEAwEEAQIBAAAAAAIAAAEJCgsMDQ4PEACx/UmM")
			So(err, ShouldBeNil)

			decodedHistory := History{
				Runs: make([]Run, 0, 100),
			}
			hs := &HistorySerializer{}
			err = hs.DecodeInto(&decodedHistory, b)
			So(err, ShouldBeNil)

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
			So(decodedHistory.Runs, ShouldResemble, expectedRuns)
		})
		Convey(`Long history`, func() {
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
			So(err, ShouldBeNil)

			decodedHistory := History{
				Runs: make([]Run, 0, 100),
			}
			hs := &HistorySerializer{}
			err = hs.DecodeInto(&decodedHistory, b)
			So(err, ShouldBeNil)
			So(decodedHistory.Runs, ShouldResemble, expectedRuns)
		})
	})
}

func TestInputBuffer(t *testing.T) {
	Convey(`Add item to input buffer`, t, func() {
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

		So(ib.IsColdBufferDirty, ShouldBeFalse)

		// Expect compaction to have no effect.
		ib.CompactIfRequired()
		So(ib.IsColdBufferDirty, ShouldBeFalse)
		So(len(ib.HotBuffer.Runs), ShouldEqual, 9)
		So(ib.HotBuffer.Runs, ShouldResemble, []Run{
			createTestRun(1, 1),
			createTestRun(1, 4),
			createTestRun(2, 2),
			createTestRun(2, 3),
			createTestRun(2, 3),
			createTestRun(3, 3),
			createTestRun(4, 5),
			createTestRun(7, 7),
			createTestRun(7, 8),
		})

		// Insert the last verdict.
		ib.InsertRun(createTestRun(6, 2))
		So(ib.IsColdBufferDirty, ShouldBeFalse)

		// Compaction should not have an effect.
		ib.CompactIfRequired()
		So(ib.IsColdBufferDirty, ShouldBeTrue)
		So(len(ib.HotBuffer.Runs), ShouldEqual, 0)
		So(len(ib.ColdBuffer.Runs), ShouldEqual, 10)
		So(ib.ColdBuffer.Runs, ShouldResemble, []Run{
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
		})

		// The pre-allocated buffer should be retained, at the same capacity.
		So(&ib.HotBuffer.Runs[0:1][0], ShouldEqual, &originalHotBuffer[0:1][0])
		So(&ib.ColdBuffer.Runs[0], ShouldEqual, &originalColdBuffer[0:1][0])
	})
	Convey(`Test result flow is efficient`, t, func() {
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
		So(coldBufferDirtyCount, ShouldBeLessThanOrEqualTo, 100)
		// Eviction is similar, but only starts once the cold buffer
		// is full.
		So(evictionCount, ShouldEqual, 90)
	})
	Convey(`Compaction should maintain order`, t, func() {
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
		So(cap(originalColdBuffer), ShouldEqual, 10)

		ib.CompactIfRequired()
		So(len(ib.HotBuffer.Runs), ShouldEqual, 0)
		So(len(ib.ColdBuffer.Runs), ShouldEqual, 10)
		So(ib.ColdBuffer.Runs, ShouldResemble, []Run{
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
		})

		// The pre-allocated buffer should be retained, at the same capacity.
		So(&ib.HotBuffer.Runs[0:1][0], ShouldEqual, &originalHotBuffer[0:1][0])
		So(&ib.ColdBuffer.Runs[0], ShouldEqual, &originalColdBuffer[0:1][0])
	})

	Convey(`Cold buffer should keep old verdicts after compaction`, t, func() {
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
		So(len(ib.HotBuffer.Runs), ShouldEqual, 0)
		So(len(ib.ColdBuffer.Runs), ShouldEqual, 7)
		So(ib.ColdBuffer.Runs, ShouldResemble, []Run{
			createTestRun(2, 1),
			createTestRun(4, 1),
			createTestRun(6, 1),
			createTestRun(7, 1),
			createTestRun(8, 1),
			createTestRun(9, 1),
			createTestRun(10, 1),
		})
	})

	Convey(`EvictBefore`, t, func() {
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
		So(cap(buffer.Runs), ShouldEqual, 5)

		Convey(`Start of slice`, func() {
			buffer.EvictBefore(0)
			So(buffer, ShouldResemble, History{
				Runs: []Run{
					createTestRun(2, 1),
					createTestRun(4, 1),
					createTestRun(6, 1),
					createTestRun(8, 1),
					createTestRun(10, 1),
				},
			})

			So(&buffer.Runs[0:1][0], ShouldEqual, &originalVerdictsBuffer[0])
			So(cap(buffer.Runs), ShouldEqual, 5)
		})
		Convey(`Middle of slice`, func() {
			buffer.EvictBefore(2)
			So(buffer, ShouldResemble, History{
				Runs: []Run{
					createTestRun(6, 1),
					createTestRun(8, 1),
					createTestRun(10, 1),
				},
			})

			// The pre-allocated buffer should be retained, at the same capacity.
			So(&buffer.Runs[0:1][0], ShouldEqual, &originalVerdictsBuffer[0])
			So(cap(buffer.Runs), ShouldEqual, 5)
		})
		Convey(`End of slice`, func() {
			buffer.EvictBefore(5)
			So(buffer, ShouldResemble, History{
				Runs: []Run{},
			})

			// The pre-allocated buffer should be retained, at the same capacity.
			So(&buffer.Runs[0:1][0], ShouldEqual, &originalVerdictsBuffer[0])
			So(cap(buffer.Runs), ShouldEqual, 5)
		})
		Convey(`Empty slice`, func() {
			buffer := History{
				Runs: []Run{},
			}
			buffer.EvictBefore(0)
			So(buffer, ShouldResemble, History{
				Runs: []Run{},
			})
		})
	})

	Convey(`Clear`, t, func() {
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

		So(ib, ShouldResemble, &Buffer{
			HotBufferCapacity: 5,
			HotBuffer: History{
				Runs: []Run{},
			},
			ColdBufferCapacity: 10,
			ColdBuffer: History{
				Runs: []Run{},
			},
		})
		// The pre-allocated buffer should be retained, at the same capacity.
		So(&ib.HotBuffer.Runs[0:1][0], ShouldEqual, &originalHotBuffer[0])
		So(&ib.ColdBuffer.Runs[0:1][0], ShouldEqual, &originalColdBuffer[0])
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
