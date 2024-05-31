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
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeAndDecode(t *testing.T) {
	Convey(`Encode and decode should return the same result`, t, func() {
		history := History{
			Verdicts: []PositionVerdict{
				{
					CommitPosition:       1345,
					IsSimpleExpectedPass: true,
					Hour:                 time.Unix(1000*3600, 0),
				},
				{
					CommitPosition:       1355,
					IsSimpleExpectedPass: false,
					Hour:                 time.Unix(1005*3600, 0),
					Details: VerdictDetails{
						IsExonerated: false,
						Runs: []Run{
							{
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
								IsDuplicate: false,
							},
							{
								Expected: ResultCounts{
									PassCount: 1,
								},
								Unexpected: ResultCounts{
									FailCount: 2,
								},
								IsDuplicate: true,
							},
						},
					},
				},
				{
					CommitPosition:       1357,
					IsSimpleExpectedPass: true,
					Hour:                 time.Unix(1003*3600, 0),
				},
				{
					CommitPosition:       1357,
					IsSimpleExpectedPass: false,
					Hour:                 time.Unix(1005*3600, 0),
					Details: VerdictDetails{
						IsExonerated: true,
						Runs: []Run{
							{
								Expected: ResultCounts{
									PassCount: 1,
								},
								Unexpected: ResultCounts{
									FailCount: 2,
								},
								IsDuplicate: true,
							},
							{
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
								IsDuplicate: false,
							},
						},
					},
				},
			},
		}

		hs := &HistorySerializer{}
		encoded := hs.Encode(history)
		decodedHistory := History{
			Verdicts: make([]PositionVerdict, 0, 100),
		}
		err := hs.DecodeInto(&decodedHistory, encoded)
		So(err, ShouldBeNil)
		So(len(decodedHistory.Verdicts), ShouldEqual, 4)
		So(decodedHistory, ShouldResemble, history)
	})

	Convey(`Encode and decode long history should not have error`, t, func() {
		history := History{}
		history.Verdicts = make([]PositionVerdict, 2000)
		for i := 0; i < 2000; i++ {
			history.Verdicts[i] = PositionVerdict{
				CommitPosition:       i,
				IsSimpleExpectedPass: false,
				Hour:                 time.Unix(int64(i*3600), 0),
				Details: VerdictDetails{
					IsExonerated: false,
					Runs: []Run{
						{
							Expected: ResultCounts{
								PassCount: 1,
							},
							Unexpected: ResultCounts{
								FailCount: 2,
							},
							IsDuplicate: false,
						},
						{
							Expected: ResultCounts{
								PassCount: 1,
							},
							Unexpected: ResultCounts{
								FailCount: 2,
							},
							IsDuplicate: false,
						},
						{
							Expected: ResultCounts{
								PassCount: 1,
							},
							Unexpected: ResultCounts{
								FailCount: 2,
							},
							IsDuplicate: false,
						},
					},
				},
			}
		}
		hs := &HistorySerializer{}
		encoded := hs.Encode(history)
		decodedHistory := History{
			Verdicts: make([]PositionVerdict, 0, 2000),
		}
		err := hs.DecodeInto(&decodedHistory, encoded)
		So(err, ShouldBeNil)
		So(len(decodedHistory.Verdicts), ShouldEqual, 2000)
		So(decodedHistory, ShouldResemble, history)
	})
}

func TestInputBuffer(t *testing.T) {
	Convey(`Add item to input buffer`, t, func() {
		ib := NewWithCapacity(10, 100)
		originalHotBuffer := ib.HotBuffer.Verdicts
		originalColdBuffer := ib.ColdBuffer.Verdicts

		// Insert 9 verdicts into hot buffer.
		ib.InsertVerdict(createTestVerdict(1, 4))
		ib.InsertVerdict(createTestVerdict(2, 2))
		ib.InsertVerdict(createTestVerdict(3, 3))
		ib.InsertVerdict(createTestVerdict(2, 3))
		ib.InsertVerdict(createTestVerdict(4, 5))
		ib.InsertVerdict(createTestVerdict(1, 1))
		ib.InsertVerdict(createTestVerdict(2, 3))
		ib.InsertVerdict(createTestVerdict(7, 8))
		ib.InsertVerdict(createTestVerdict(7, 7))
		So(ib.IsColdBufferDirty, ShouldBeFalse)

		// Expect compaction to have no effect.
		ib.CompactIfRequired()
		So(ib.IsColdBufferDirty, ShouldBeFalse)
		So(len(ib.HotBuffer.Verdicts), ShouldEqual, 9)
		So(ib.HotBuffer.Verdicts, ShouldResemble, []PositionVerdict{
			createTestVerdict(1, 1),
			createTestVerdict(1, 4),
			createTestVerdict(2, 2),
			createTestVerdict(2, 3),
			createTestVerdict(2, 3),
			createTestVerdict(3, 3),
			createTestVerdict(4, 5),
			createTestVerdict(7, 7),
			createTestVerdict(7, 8),
		})

		// Insert the last verdict.
		ib.InsertVerdict(createTestVerdict(6, 2))
		So(ib.IsColdBufferDirty, ShouldBeFalse)

		// Compaction should have an effect.
		ib.CompactIfRequired()
		So(ib.IsColdBufferDirty, ShouldBeTrue)
		So(len(ib.HotBuffer.Verdicts), ShouldEqual, 0)
		So(len(ib.ColdBuffer.Verdicts), ShouldEqual, 10)
		So(ib.ColdBuffer.Verdicts, ShouldResemble, []PositionVerdict{
			createTestVerdict(1, 1),
			createTestVerdict(1, 4),
			createTestVerdict(2, 2),
			createTestVerdict(2, 3),
			createTestVerdict(2, 3),
			createTestVerdict(3, 3),
			createTestVerdict(4, 5),
			createTestVerdict(6, 2),
			createTestVerdict(7, 7),
			createTestVerdict(7, 8),
		})

		// The pre-allocated buffer should be retained, at the same capacity.
		So(&ib.HotBuffer.Verdicts[0:1][0], ShouldEqual, &originalHotBuffer[0:1][0])
		So(&ib.ColdBuffer.Verdicts[0], ShouldEqual, &originalColdBuffer[0:1][0])
	})
	Convey(`Test result flow is efficient`, t, func() {
		ib := NewWithCapacity(10, 100)
		coldBufferDirtyCount := 0
		evictionCount := 0
		for i := 0; i < 1000; i++ {
			ib.InsertVerdict(createTestVerdict(i, i))
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
				Verdicts: []PositionVerdict{
					createTestVerdict(1, 1),
					createTestVerdict(3, 1),
					createTestVerdict(5, 1),
					createTestVerdict(7, 1),
					createTestVerdict(9, 1),
				},
			},
			ColdBufferCapacity: 10,
			ColdBuffer: History{
				// Allocate with capacity 10 so there is enough
				// space to do an in-place compaction.
				Verdicts: append(make([]PositionVerdict, 0, 10), []PositionVerdict{
					createTestVerdict(2, 1),
					createTestVerdict(4, 1),
					createTestVerdict(6, 1),
					createTestVerdict(8, 1),
					createTestVerdict(10, 1),
				}...),
			},
		}
		originalHotBuffer := ib.HotBuffer.Verdicts
		originalColdBuffer := ib.ColdBuffer.Verdicts
		So(cap(originalColdBuffer), ShouldEqual, 10)

		ib.CompactIfRequired()
		So(len(ib.HotBuffer.Verdicts), ShouldEqual, 0)
		So(len(ib.ColdBuffer.Verdicts), ShouldEqual, 10)
		So(ib.ColdBuffer.Verdicts, ShouldResemble, []PositionVerdict{
			createTestVerdict(1, 1),
			createTestVerdict(2, 1),
			createTestVerdict(3, 1),
			createTestVerdict(4, 1),
			createTestVerdict(5, 1),
			createTestVerdict(6, 1),
			createTestVerdict(7, 1),
			createTestVerdict(8, 1),
			createTestVerdict(9, 1),
			createTestVerdict(10, 1),
		})

		// The pre-allocated buffer should be retained, at the same capacity.
		So(&ib.HotBuffer.Verdicts[0:1][0], ShouldEqual, &originalHotBuffer[0:1][0])
		So(&ib.ColdBuffer.Verdicts[0], ShouldEqual, &originalColdBuffer[0:1][0])
	})

	Convey(`Cold buffer should keep old verdicts after compaction`, t, func() {
		ib := Buffer{
			HotBufferCapacity: 2,
			HotBuffer: History{
				Verdicts: []PositionVerdict{
					createTestVerdict(7, 1),
					createTestVerdict(9, 1),
				},
			},
			ColdBufferCapacity: 5,
			ColdBuffer: History{
				Verdicts: []PositionVerdict{
					createTestVerdict(2, 1),
					createTestVerdict(4, 1),
					createTestVerdict(6, 1),
					createTestVerdict(8, 1),
					createTestVerdict(10, 1),
				},
			},
		}

		ib.CompactIfRequired()
		So(len(ib.HotBuffer.Verdicts), ShouldEqual, 0)
		So(len(ib.ColdBuffer.Verdicts), ShouldEqual, 7)
		So(ib.ColdBuffer.Verdicts, ShouldResemble, []PositionVerdict{
			createTestVerdict(2, 1),
			createTestVerdict(4, 1),
			createTestVerdict(6, 1),
			createTestVerdict(7, 1),
			createTestVerdict(8, 1),
			createTestVerdict(9, 1),
			createTestVerdict(10, 1),
		})
	})

	Convey(`EvictBefore`, t, func() {
		buffer := History{
			Verdicts: []PositionVerdict{
				createTestVerdict(2, 1),
				createTestVerdict(4, 1),
				createTestVerdict(6, 1),
				createTestVerdict(8, 1),
				createTestVerdict(10, 1),
			},
		}
		originalVerdictsBuffer := buffer.Verdicts
		So(cap(buffer.Verdicts), ShouldEqual, 5)

		Convey(`Start of slice`, func() {
			buffer.EvictBefore(0)
			So(buffer, ShouldResemble, History{
				Verdicts: []PositionVerdict{
					createTestVerdict(2, 1),
					createTestVerdict(4, 1),
					createTestVerdict(6, 1),
					createTestVerdict(8, 1),
					createTestVerdict(10, 1),
				},
			})

			So(&buffer.Verdicts[0:1][0], ShouldEqual, &originalVerdictsBuffer[0])
			So(cap(buffer.Verdicts), ShouldEqual, 5)
		})
		Convey(`Middle of slice`, func() {
			buffer.EvictBefore(2)
			So(buffer, ShouldResemble, History{
				Verdicts: []PositionVerdict{
					createTestVerdict(6, 1),
					createTestVerdict(8, 1),
					createTestVerdict(10, 1),
				},
			})

			// The pre-allocated buffer should be retained, at the same capacity.
			So(&buffer.Verdicts[0:1][0], ShouldEqual, &originalVerdictsBuffer[0])
			So(cap(buffer.Verdicts), ShouldEqual, 5)
		})
		Convey(`End of slice`, func() {
			buffer.EvictBefore(5)
			So(buffer, ShouldResemble, History{
				Verdicts: []PositionVerdict{},
			})

			// The pre-allocated buffer should be retained, at the same capacity.
			So(&buffer.Verdicts[0:1][0], ShouldEqual, &originalVerdictsBuffer[0])
			So(cap(buffer.Verdicts), ShouldEqual, 5)
		})
		Convey(`Empty slice`, func() {
			buffer := History{
				Verdicts: []PositionVerdict{},
			}
			buffer.EvictBefore(0)
			So(buffer, ShouldResemble, History{
				Verdicts: []PositionVerdict{},
			})
		})
	})

	Convey(`Clear`, t, func() {
		ib := NewWithCapacity(5, 10)
		ib.HotBuffer.Verdicts = append(ib.HotBuffer.Verdicts, []PositionVerdict{
			createTestVerdict(1, 1),
			createTestVerdict(3, 1),
			createTestVerdict(5, 1),
			createTestVerdict(7, 1),
			createTestVerdict(9, 1),
		}...)
		ib.ColdBuffer.Verdicts = append(ib.ColdBuffer.Verdicts, []PositionVerdict{
			createTestVerdict(2, 1),
			createTestVerdict(4, 1),
			createTestVerdict(6, 1),
			createTestVerdict(8, 1),
			createTestVerdict(10, 1),
		}...)
		originalHotBuffer := ib.HotBuffer.Verdicts
		originalColdBuffer := ib.ColdBuffer.Verdicts

		ib.Clear()

		So(ib, ShouldResemble, &Buffer{
			HotBufferCapacity: 5,
			HotBuffer: History{
				Verdicts: []PositionVerdict{},
			},
			ColdBufferCapacity: 10,
			ColdBuffer: History{
				Verdicts: []PositionVerdict{},
			},
		})
		// The pre-allocated buffer should be retained, at the same capacity.
		So(&ib.HotBuffer.Verdicts[0:1][0], ShouldEqual, &originalHotBuffer[0])
		So(&ib.ColdBuffer.Verdicts[0:1][0], ShouldEqual, &originalColdBuffer[0])
	})
}

func createTestVerdict(pos int, hour int) PositionVerdict {
	return PositionVerdict{
		CommitPosition:       pos,
		IsSimpleExpectedPass: true,
		Hour:                 time.Unix(int64(3600*hour), 0),
	}
}
