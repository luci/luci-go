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

// Package inputbuffer handles the input buffer of change point analysis.
package inputbuffer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/span"
)

const (
	// The version of the encoding to encode the run history.
	EncodingVersion = 3
	// Capacity of the hot buffer, i.e. how many runs it can hold.
	DefaultHotBufferCapacity = 100
	// Capacity of the cold buffer, i.e. how many runs it can hold.
	DefaultColdBufferCapacity = 2000
	// RunsInsertedHint is the number of test expected
	// to be inserted in one usage of the input buffer. Buffers
	// will be allocated assuming this is the maximum number
	// inserted, if the actual number is higher, it may trigger
	// additional memory allocations.
	//
	// Currently a value of 100 is used as ingestion process ingests one
	// verdict at a time, which may consist of up to 100 runs.
	RunsInsertedHint = 100
)

type Buffer struct {
	// Capacity of the hot buffer. If it is full, the content will be written
	// into the cold buffer.
	HotBufferCapacity int
	HotBuffer         History
	// Capacity of the cold buffer.
	ColdBufferCapacity int
	ColdBuffer         History
	// IsColdBufferDirty will be set to true if the cold buffer is dirty.
	// This means we need to write the cold buffer to Spanner.
	IsColdBufferDirty bool
}

type History struct {
	// Runs, sorted by commit position (oldest first), and
	// then result time (oldest first).
	Runs []Run
}

// Run represents all tries of a test in a single invocation.
type Run struct {
	// The source position of the run.
	//
	// This abstracts away the underlying source control system and
	// identifies the version of code sources (e.g. git commit or repo snapshot)
	// tested with a number.
	//
	// A larger value indicates "newer" sources, an equal value means
	// "equal" sources, and a small number means "older" sources.
	// See pbutil.SourcePosition for precise definition.
	SourcePosition int64
	// The partition time for this run, truncated to the nearest hour.
	Hour time.Time
	// Counts for expected results.
	Expected ResultCounts
	// Counts for unexpected results.
	Unexpected ResultCounts
}

// IsSimpleExpectedPass returns whether this run is a simple expected pass run or not.
// A simple expected pass run has only one test result, which is expected pass.
// This represents approximately ~97% of runs at time of writing and
// is a property levereged in the encoding of the input buffer.
func (r Run) IsSimpleExpectedPass() bool {
	return r.Expected == ResultCounts{PassCount: 1} && r.Unexpected == ResultCounts{}
}

type ResultCounts struct {
	// Number of passed result.
	PassCount int
	// Number of failed result.
	FailCount int
	// Number of crashed result.
	CrashCount int
	// Number of aborted result.
	AbortCount int
}

func (r ResultCounts) Count() int {
	return r.PassCount + r.FailCount + r.CrashCount + r.AbortCount
}

// New allocates an empty input buffer with default capacity.
func New() *Buffer {
	return NewWithCapacity(DefaultHotBufferCapacity, DefaultColdBufferCapacity)
}

// NewWithCapacity allocates an empty input buffer with the given capacity.
func NewWithCapacity(hotBufferCapacity, coldBufferCapacity int) *Buffer {
	return &Buffer{
		// HotBufferCapacity is a hard limit on the number of runs
		// stored in Spanner, but only a soft limit during processing.
		// After new runs are ingested, but before eviction is
		// considered, the limit can be exceeded.
		HotBufferCapacity: hotBufferCapacity,
		HotBuffer:         History{Runs: make([]Run, 0, hotBufferCapacity+RunsInsertedHint)},
		// ColdBufferCapacity is a hard limit on the number of runs
		// stored in Spanner, but only a soft limit during processing.
		// After new runs are ingested, but before eviction is
		// considered, the limit can be exceeded (due to the new
		// runs or due to compaction from hot buffer to cold buffer).
		ColdBufferCapacity: coldBufferCapacity,
		ColdBuffer:         History{Runs: make([]Run, 0, coldBufferCapacity+hotBufferCapacity+RunsInsertedHint)},
	}
}

// Copy makes a deep copy of the input buffer.
func (ib *Buffer) Copy() *Buffer {
	return &Buffer{
		HotBufferCapacity:  ib.HotBufferCapacity,
		HotBuffer:          ib.HotBuffer.Copy(),
		ColdBufferCapacity: ib.ColdBufferCapacity,
		ColdBuffer:         ib.ColdBuffer.Copy(),
		IsColdBufferDirty:  ib.IsColdBufferDirty,
	}
}

// Copy makes a deep copy of the History.
func (h History) Copy() History {
	// Make a deep copy of runs.
	runsCopy := make([]Run, len(h.Runs), cap(h.Runs))
	copy(runsCopy, h.Runs)

	return History{Runs: runsCopy}
}

// EvictBefore removes all runs prior to (but not including) the given index.
//
// This will modify the runs buffer in-place, existing subslices
// should be treated as invalid following this operation.
func (h *History) EvictBefore(index int) {
	// Instead of the obvious:
	// h.Runs = h.Runs[index:]
	// We shuffle all items forward by index so that we retain
	// the same underlying Runs buffer, with the same capacity.

	// Shuffle all items forward by 'index'.
	for i := index; i < len(h.Runs); i++ {
		h.Runs[i-index] = h.Runs[i]
	}
	h.Runs = h.Runs[:len(h.Runs)-index]
}

// Clear resets the input buffer to an empty state, similar to
// its state after New().
func (ib *Buffer) Clear() {
	if cap(ib.HotBuffer.Runs) < ib.HotBufferCapacity+RunsInsertedHint {
		// Indicates a logic error if someone discarded part of the
		// originally allocated buffer.
		panic("buffer capacity unexpectedly modified")
	}
	if cap(ib.ColdBuffer.Runs) < ib.ColdBufferCapacity+ib.HotBufferCapacity+RunsInsertedHint {
		// Indicates a logic error if someone discarded part of the
		// originally allocated buffer.
		panic("buffer capacity unexpectedly modified")
	}
	ib.HotBuffer.Runs = ib.HotBuffer.Runs[:0]
	ib.ColdBuffer.Runs = ib.ColdBuffer.Runs[:0]
	ib.IsColdBufferDirty = false
}

// InsertRun inserts a new run into the input buffer.
//
// The run is always inserted into the hot buffer, run CompactIfRequired
// as required after all runs have been inserted.
func (ib *Buffer) InsertRun(r Run) {
	// Find the position to insert the run.
	// As the new run is likely to have the latest commit position, we
	// will iterate backwards from the end of the slice.
	runs := ib.HotBuffer.Runs
	pos := len(runs)
	for ; pos > 0; pos-- {
		if compareRun(&r, &runs[pos-1]) > 0 {
			// run is after the run at position-1,
			// so insert at position.
			break
		}
	}

	// Shuffle all runs in runs[pos:] forwards
	// to create a spot at the insertion position.
	// (We want to avoid allocating a new slice.)
	runs = append(runs, Run{})
	for i := len(runs) - 1; i > pos; i-- {
		runs[i] = runs[i-1]
	}
	runs[pos] = r
	ib.HotBuffer.Runs = runs
}

// CompactIfRequired moves the content from the hot buffer to the cold buffer,
// if:
//   - the hot buffer is at capacity, or
//   - the cold buffer is at capacity and an eviction from the cold buffer
//     is going to be required (this case is important for buffers
//     stored in legacy data formats, where the buffer may immediately be
//     beyond its capacity). Note that eviction from the cold buffer requires
//     the hot buffer to be empty.
//
// If a compaction occurs, the IsColdBufferDirty flag will be set to true,
// implying that the cold buffer content needs to be written to Spanner.
//
// Note: It is possible that the cold buffer overflows after the compaction,
// i.e., len(ColdBuffer.Runs) > ColdBufferCapacity.
// This needs to be handled separately.
func (ib *Buffer) CompactIfRequired() {
	if len(ib.HotBuffer.Runs) < ib.HotBufferCapacity && len(ib.ColdBuffer.Runs) <= ib.ColdBufferCapacity {
		// No compaction required.
		return
	}
	ib.IsColdBufferDirty = true

	var merged []*Run
	ib.MergeBuffer(&merged)
	ib.HotBuffer.Runs = ib.HotBuffer.Runs[:0]

	// Copy the merged runs to the ColdBuffer instead of assigning
	// the merged buffer, so that we keep the same pre-allocated buffer.
	ib.ColdBuffer.Runs = ib.ColdBuffer.Runs[:0]
	ib.ColdBuffer.Runs = append(ib.ColdBuffer.Runs, copyAndFlattenRuns(merged)...)
}

// MergeBuffer merges the runs of the hot buffer and the cold buffer
// into the provided slice, resizing it if necessary.
// The returned slice will be sorted by commit position (oldest first), and
// then by result time (oldest first).
// Any changes to the input buffers will invalidate the slice, as it
// is built via pointers into the input buffer.
func (ib *Buffer) MergeBuffer(destination *[]*Run) {
	// Because the hot buffer and cold buffer are both sorted, we can simply use
	// a single merge to merge the 2 buffers.
	hRuns := ib.HotBuffer.Runs
	cRuns := ib.ColdBuffer.Runs

	if *destination == nil {
		*destination = make([]*Run, 0, ib.ColdBufferCapacity+ib.HotBufferCapacity+RunsInsertedHint)
	}
	MergeOrderedRuns(hRuns, cRuns, destination)
}

// EvictionRange returns the part that should be evicted from cold buffer, due
// to overflow.
// Note: we never evict from the hot buffer due to overflow. Overflow from the
// hot buffer should cause compaction to the cold buffer instead.
// Returns:
// - a boolean (shouldEvict) to indicated if an eviction should occur.
// - a number (endIndex) for the eviction. The eviction will occur for range
// [0, endIndex (inclusively)].
// Note that eviction can only occur after a compaction from hot buffer to cold
// buffer. It means the hot buffer is empty, and the cold buffer overflows.
func (ib *Buffer) EvictionRange() (shouldEvict bool, endIndex int) {
	if len(ib.ColdBuffer.Runs) <= ib.ColdBufferCapacity {
		return false, 0
	}
	if len(ib.HotBuffer.Runs) > 0 {
		panic("hot buffer is not empty during eviction")
	}
	return true, len(ib.ColdBuffer.Runs) - ib.ColdBufferCapacity - 1
}

func (ib *Buffer) Size() int {
	return len(ib.ColdBuffer.Runs) + len(ib.HotBuffer.Runs)
}

// HistorySerializer provides methods to decode and encode History objects.
// Methods on a given instance are only safe to call on one goroutine at
// a time.
type HistorySerializer struct {
	// A preallocated buffer to store encoded, uncompressed runs.
	// Avoids needing to allocate a new buffer for every decode/encode
	// operation, with consequent heap requirements and GC churn.
	tempBuf []byte
}

// ensureAndClearBuf returns a temporary buffer with suitable capacity
// and zero length.
func (hs *HistorySerializer) ensureAndClearBuf() {
	if hs.tempBuf == nil {
		// At most the history will have 2000 runs (cold buffer).
		// Most runs will be simple expected run, so 30,000 is probably fine
		// for most cases.
		// In case 30,000 bytes is not enough, Encode() or Decode()
		// will resize to an appropriate size.
		hs.tempBuf = make([]byte, 0, 30000)
	} else {
		hs.tempBuf = hs.tempBuf[:0]
	}
}

// Encode uses varint encoding to encode history into a byte array.
// See go/luci-test-variant-analysis-design for details.
func (hs *HistorySerializer) Encode(history History) []byte {
	hs.ensureAndClearBuf()
	buf := hs.tempBuf
	buf = binary.AppendUvarint(buf, uint64(EncodingVersion))
	buf = binary.AppendUvarint(buf, uint64(len(history.Runs)))

	var lastPosition uint64
	var lastHourNumber int64
	for i := 0; i < len(history.Runs); i++ {
		run := &history.Runs[i]

		// We encode the relative deltaPosition between the current run and the
		// previous runs.
		deltaPosition := uint64(run.SourcePosition) - lastPosition
		// Shift two bits to leave space for flags.
		// The last bit is set to 1 for non-simple runs.
		// The second last bit is reserved for future use.
		deltaPosition = deltaPosition << 2
		isSimpleExpectedPass := run.IsSimpleExpectedPass()
		if !isSimpleExpectedPass {
			// Set the last bit to 1 if it is not a single expected passing run.
			deltaPosition |= 1
		}
		buf = binary.AppendUvarint(buf, deltaPosition)
		lastPosition = uint64(run.SourcePosition)

		// Encode the "relative" hour.
		// Note that the relative hour may be positive or negative. So we are encoding
		// it as varint.
		hourNumber := run.Hour.Unix() / 3600
		deltaHour := hourNumber - lastHourNumber
		buf = binary.AppendVarint(buf, deltaHour)
		lastHourNumber = hourNumber

		// Encode the run details, only if not simple expected passing run.
		if !isSimpleExpectedPass {
			buf = appendRunDetailedCounts(buf, run.Expected, run.Unexpected)
		}
	}
	// It is possible the size of buf was increased in this method.
	// If so, keep that larger buf for future encodings.
	hs.tempBuf = buf

	// Use zstd to compress the result. Note that the buffer returned
	// by Compress is always different to hs.tempBuf, so tempBuf does
	// not escape.
	return span.Compress(buf)
}

// DecodeInto decodes the runs in buf, populating the history object.
func (hs *HistorySerializer) DecodeInto(history *History, buf []byte) error {
	// Clear existing runs to avoid state from a previous
	// decoding leaking.
	runs := history.Runs[:0]

	var err error
	hs.ensureAndClearBuf()
	// If it is possible hs.tempBuf was resized to be able to accept
	// all the decompressed content. If so, keep it, so we can use
	// the larger buf for future decodings.
	hs.tempBuf, err = span.Decompress(buf, hs.tempBuf)
	if err != nil {
		return errors.Fmt("decompress error: %w", err)
	}
	reader := bytes.NewReader(hs.tempBuf)

	// Read version.
	version, err := binary.ReadUvarint(reader)
	if err != nil {
		return errors.Fmt("read version: %w", err)
	}
	if version == LegacyV2EncodingVersion {
		return LegacyV2DecodeInto(history, reader)
	}
	if version != EncodingVersion {
		return fmt.Errorf("version mismatched: got version %d, want %d", version, EncodingVersion)
	}

	// Read run count.
	nRuns, err := binary.ReadUvarint(reader)
	if err != nil {
		return errors.Fmt("read number of runs: %w", err)
	}
	if nRuns > uint64(cap(runs)) {
		// The caller has allocated an inappropriately sized buffer.
		return errors.Fmt("found %v runs to decode, but capacity is only %v", nRuns, cap(runs))
	}

	for i := 0; i < int(nRuns); i++ {
		// This loop is a 'hot' code path that may execute ~2000 times
		// for each of the ~10,000 test verdicts ingested per page.

		// Benchmarks indicate it is faster to append a new run first
		// and modify it in place than it is to modify one in a local
		// variable and then append it to the list.
		runs = append(runs, Run{})
		run := &runs[i]

		// Get the commit position for the runs, and if the run is simple
		// expected.
		posSim, err := binary.ReadUvarint(reader)
		if err != nil {
			return errors.Fmt("read run position and flags: %w", err)
		}
		deltaPos, isSimpleExpectedPass := decodePositionAndFlags(posSim)

		if i == 0 {
			// For the first run, deltaPos is the absolute commit position.
			run.SourcePosition = deltaPos
		} else {
			// For later runs, deltaPos records the relative difference.
			run.SourcePosition = runs[i-1].SourcePosition + deltaPos
		}

		// Get the hour.
		deltaHour, err := binary.ReadVarint(reader)
		if err != nil {
			return errors.Fmt("read delta hour: %w", err)
		}
		if i == 0 {
			// For the first run, deltaHour is the absolute hour.
			run.Hour = time.Unix(deltaHour*3600, 0)
		} else {
			// For later runs, deltaHour records the relative difference.
			secs := runs[i-1].Hour.Unix()
			run.Hour = time.Unix(secs+deltaHour*3600, 0)
		}

		if isSimpleExpectedPass {
			run.Expected.PassCount = 1
		} else {
			// Read the detailed run counts.
			var err error
			run.Expected, run.Unexpected, err = readRunDetailedCounts(reader)
			if err != nil {
				return errors.Fmt("read run details: %w", err)
			}
		}
	}
	history.Runs = runs

	return err
}

func readRunDetailedCounts(reader *bytes.Reader) (expected, unexpected ResultCounts, err error) {
	// Read expected passed count.
	expectedPassedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read expected passed count: %w", err)
	}
	expected.PassCount = int(expectedPassedCount)

	// Read expected failed count.
	expectedFailedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read expected failed count: %w", err)
	}
	expected.FailCount = int(expectedFailedCount)

	// Read expected crashed count.
	expectedCrashedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read expected crashed count: %w", err)
	}
	expected.CrashCount = int(expectedCrashedCount)

	// Read expected aborted count.
	expectedAbortedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read expected aborted count: %w", err)
	}
	expected.AbortCount = int(expectedAbortedCount)

	// Read unexpected passed count.
	unexpectedPassedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read unexpected passed count: %w", err)
	}
	unexpected.PassCount = int(unexpectedPassedCount)

	// Read unexpected failed count.
	unexpectedFailedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read unexpected failed count: %w", err)
	}
	unexpected.FailCount = int(unexpectedFailedCount)

	// Read unexpected crashed count.
	unexpectedCrashedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read unexpected crashed count: %w", err)
	}
	unexpected.CrashCount = int(unexpectedCrashedCount)

	// Read unexpected aborted count.
	unexpectedAbortedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return ResultCounts{}, ResultCounts{}, errors.Fmt("read unexpected aborted count: %w", err)
	}
	unexpected.AbortCount = int(unexpectedAbortedCount)

	return expected, unexpected, nil
}

func appendRunDetailedCounts(result []byte, expected, unexpected ResultCounts) []byte {
	result = binary.AppendUvarint(result, uint64(expected.PassCount))
	result = binary.AppendUvarint(result, uint64(expected.FailCount))
	result = binary.AppendUvarint(result, uint64(expected.CrashCount))
	result = binary.AppendUvarint(result, uint64(expected.AbortCount))
	result = binary.AppendUvarint(result, uint64(unexpected.PassCount))
	result = binary.AppendUvarint(result, uint64(unexpected.FailCount))
	result = binary.AppendUvarint(result, uint64(unexpected.CrashCount))
	result = binary.AppendUvarint(result, uint64(unexpected.AbortCount))
	return result
}

// decodePositionAndFlags decodes the value posFlags and returns 2 values.
// 1. The (delta) commit position.
// 2. Whether the run is a simple expected pass.
func decodePositionAndFlags(posFlags uint64) (deltaPos int64, isSimple bool) {
	isSimple = false
	// The last bit of posFlags is set to 1 if the run is NOT a simple expected pass.
	if (posFlags & 1) == 0 {
		isSimple = true
	}
	// The second last bit of posFlags is reserved for future use.

	deltaPos = int64(posFlags >> 2)
	return deltaPos, isSimple
}

func boolToUInt64(b bool) uint64 {
	result := 0
	if b {
		result = 1
	}
	return uint64(result)
}

func uInt64ToBool(u uint64) bool {
	return u == 1
}

// compareRun returns 1 if v1 is later than v2, -1 if v1 is earlier than
// v2, and 0 if they are at the same time.
// The comparision is done on commit position, then on hour.
func compareRun(v1 *Run, v2 *Run) int {
	if v1.SourcePosition > v2.SourcePosition {
		return 1
	}
	if v1.SourcePosition < v2.SourcePosition {
		return -1
	}
	if v1.Hour.After(v2.Hour) {
		return 1
	}
	if v1.Hour.Before(v2.Hour) {
		return -1
	}
	return 0
}
