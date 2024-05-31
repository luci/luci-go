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
	// The version of the encoding to encode the verdict history.
	EncodingVersion = 2
	// Capacity of the hot buffer, i.e. how many verdicts it can hold.
	DefaultHotBufferCapacity = 100
	// Capacity of the cold buffer, i.e. how many verdicts it can hold.
	DefaultColdBufferCapacity = 2000
	// VerdictsInsertedHint is the number of verdicts expected
	// to be inserted in one usage of the input buffer. Buffers
	// will be allocated assuming this is the maximum number
	// inserted, if the actual number is higher, it may trigger
	// additional memory allocations.
	//
	// Currently a value of 1 is used as ingestion process ingests one
	// invocation at a time.
	VerdictsInsertedHint = 1
)

type Buffer struct {
	// Capacity of the hot buffer. If it is full, the content will be written
	// into the cold buffer.
	HotBufferCapacity int
	HotBuffer         History
	// Capacity of the cold buffer.
	ColdBufferCapacity int
	ColdBuffer         History
	// IsColdBufferDirty will be set to 1 if the cold buffer is dirty.
	// This means we need to write the cold buffer to Spanner.
	IsColdBufferDirty bool
}

type History struct {
	// Verdicts, sorted by commit position (oldest first), and
	// then result time (oldest first).
	Verdicts []PositionVerdict
}

type PositionVerdict struct {
	// The commit position for the verdict.
	CommitPosition int
	// Denotes whether this verdict is a simple expected pass verdict or not.
	// A simple expected pass verdict has only one test result, which is expected pass.
	IsSimpleExpectedPass bool
	// The partition time that this PositionVerdict was ingested.
	// When stored, it is truncated to the nearest hour.
	Hour time.Time
	// The details of the verdict.
	Details VerdictDetails
}

type VerdictDetails struct {
	// Whether a verdict is exonerated or not.
	IsExonerated bool
	// Details of the runs in the verdict.
	Runs []Run
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

type Run struct {
	// Counts for expected results.
	Expected ResultCounts
	// Counts for unexpected results.
	Unexpected ResultCounts
	// Whether this run is a duplicate run.
	IsDuplicate bool
}

func (r ResultCounts) Count() int {
	return r.PassCount + r.FailCount + r.CrashCount + r.AbortCount
}

type MergedInputBuffer struct {
	inputBuffer *Buffer
	// Buffer contains the merged verdicts.
	Buffer History
}

// New allocates an empty input buffer with default capacity.
func New() *Buffer {
	return NewWithCapacity(DefaultHotBufferCapacity, DefaultColdBufferCapacity)
}

// NewWithCapacity allocates an empty input buffer with the given capacity.
func NewWithCapacity(hotBufferCapacity, coldBufferCapacity int) *Buffer {
	return &Buffer{
		HotBufferCapacity: hotBufferCapacity,
		// HotBufferCapacity is a hard limit on the number of verdicts
		// stored in Spanner, but only a soft limit during processing.
		// After new verdicts are ingested, but before eviction is
		// considered, the limit can be exceeded.
		HotBuffer:          History{Verdicts: make([]PositionVerdict, 0, hotBufferCapacity+VerdictsInsertedHint)},
		ColdBufferCapacity: coldBufferCapacity,
		// ColdBufferCapacity is a hard limit on the number of verdicts
		// stored in Spanner, but only a soft limit during processing.
		// After new verdicts are ingested, but before eviction is
		// considered, the limit can be exceeded (due to the new
		// verdicts or due to compaction from hot buffer to cold buffer).
		ColdBuffer: History{Verdicts: make([]PositionVerdict, 0, coldBufferCapacity+hotBufferCapacity+VerdictsInsertedHint)},
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
	// Make a deep copy of verdicts.
	verdictsCopy := make([]PositionVerdict, len(h.Verdicts), cap(h.Verdicts))
	copy(verdictsCopy, h.Verdicts)

	// Including the nested runs slice.
	for i, v := range verdictsCopy {
		if v.Details.Runs != nil {
			runsCopy := make([]Run, len(v.Details.Runs))
			copy(runsCopy, v.Details.Runs)
			verdictsCopy[i].Details.Runs = runsCopy
		}
	}

	return History{Verdicts: verdictsCopy}
}

// EvictBefore removes all verdicts prior (but not including) the given index.
//
// This will modify the verdicts buffer in-place, existing subslices
// should be treated as invalid following this operation.
func (h *History) EvictBefore(index int) {
	// Instead of the obvious:
	// h.Verdicts = h.Verdicts[index:]
	// We shuffle all items forward by index so that we retain
	// the same underlying Verdicts buffer, with the same capacity.

	// Shuffle all items forward by 'index'.
	for i := index; i < len(h.Verdicts); i++ {
		h.Verdicts[i-index] = h.Verdicts[i]
	}
	h.Verdicts = h.Verdicts[:len(h.Verdicts)-index]
}

// Clear resets the input buffer to an empty state, similar to
// its state after New().
func (ib *Buffer) Clear() {
	if cap(ib.HotBuffer.Verdicts) < ib.HotBufferCapacity+VerdictsInsertedHint {
		// Indicates a logic error if someone discarded part of the
		// originally allocated buffer.
		panic("buffer capacity unexpectedly modified")
	}
	if cap(ib.ColdBuffer.Verdicts) < ib.ColdBufferCapacity+ib.HotBufferCapacity+VerdictsInsertedHint {
		// Indicates a logic error if someone discarded part of the
		// originally allocated buffer.
		panic("buffer capacity unexpectedly modified")
	}
	ib.HotBuffer.Verdicts = ib.HotBuffer.Verdicts[:0]
	ib.ColdBuffer.Verdicts = ib.ColdBuffer.Verdicts[:0]
	ib.IsColdBufferDirty = false
}

// InsertVerdict inserts a new verdict into the input buffer.
// It will first try to insert in the hot buffer, and if the hot buffer is full
// as the result of the insert, then a compaction will occur.
// If a compaction occurs, the IsColdBufferDirty flag will be set to true,
// implying that the cold buffer content needs to be written to Spanner.
func (ib *Buffer) InsertVerdict(v PositionVerdict) {
	// Find the position to insert the verdict.
	// As the new verdict is likely to have the latest commit position, we
	// will iterate backwards from the end of the slice.
	verdicts := ib.HotBuffer.Verdicts
	pos := len(verdicts)
	for ; pos > 0; pos-- {
		if compareVerdict(v, verdicts[pos-1]) == 1 {
			// verdict is after the verdict at position-1,
			// so insert at position.
			break
		}
	}

	// Shuffle all verdicts in verdicts[pos:] forwards
	// to create a spot at the insertion position.
	// (We want to avoid allocating a new slice.)
	verdicts = append(verdicts, PositionVerdict{})
	for i := len(verdicts) - 1; i > pos; i-- {
		verdicts[i] = verdicts[i-1]
	}
	verdicts[pos] = v
	ib.HotBuffer.Verdicts = verdicts
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
// i.e., len(ColdBuffer.Verdicts) > ColdBufferCapacity.
// This needs to be handled separately.
func (ib *Buffer) CompactIfRequired() {
	if len(ib.HotBuffer.Verdicts) < ib.HotBufferCapacity && len(ib.ColdBuffer.Verdicts) <= ib.ColdBufferCapacity {
		// No compaction required.
		return
	}
	ib.IsColdBufferDirty = true

	var merged []PositionVerdict
	ib.MergeBuffer(&merged)
	ib.HotBuffer.Verdicts = ib.HotBuffer.Verdicts[:0]

	// Copy the merged verdicts to the ColdBuffer instead of assigning
	// the merged buffer, so that we keep the same pre-allocated buffer.
	ib.ColdBuffer.Verdicts = ib.ColdBuffer.Verdicts[:0]
	for _, v := range merged {
		ib.ColdBuffer.Verdicts = append(ib.ColdBuffer.Verdicts, v)
	}
}

// MergeBuffer merges the verdicts of the hot buffer and the cold buffer
// into the provided slice, resizing it if necessary.
// The returned slice will be sorted by commit position (oldest first), and
// then by result time (oldest first).
func (ib *Buffer) MergeBuffer(destination *[]PositionVerdict) {
	// Because the hot buffer and cold buffer are both sorted, we can simply use
	// a single merge to merge the 2 buffers.
	hVerdicts := ib.HotBuffer.Verdicts
	cVerdicts := ib.ColdBuffer.Verdicts

	if *destination == nil {
		*destination = make([]PositionVerdict, 0, ib.ColdBufferCapacity+ib.HotBufferCapacity+VerdictsInsertedHint)
	}

	// Reset destination slice to zero length.
	merged := (*destination)[:0]

	hPos := 0
	cPos := 0
	for hPos < len(hVerdicts) && cPos < len(cVerdicts) {
		cmp := compareVerdict(hVerdicts[hPos], cVerdicts[cPos])
		// Item in hot buffer is strictly older.
		if cmp == -1 {
			merged = append(merged, hVerdicts[hPos])
			hPos++
		} else {
			merged = append(merged, cVerdicts[cPos])
			cPos++
		}
	}

	// Add the remaining items.
	for ; hPos < len(hVerdicts); hPos++ {
		merged = append(merged, hVerdicts[hPos])
	}
	for ; cPos < len(cVerdicts); cPos++ {
		merged = append(merged, cVerdicts[cPos])
	}

	*destination = merged
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
	if len(ib.ColdBuffer.Verdicts) <= ib.ColdBufferCapacity {
		return false, 0
	}
	if len(ib.HotBuffer.Verdicts) > 0 {
		panic("hot buffer is not empty during eviction")
	}
	return true, len(ib.ColdBuffer.Verdicts) - ib.ColdBufferCapacity - 1
}

func (ib *Buffer) Size() int {
	return len(ib.ColdBuffer.Verdicts) + len(ib.HotBuffer.Verdicts)
}

// HistorySerializer provides methods to decode and encode History objects.
// Methods on a given instance are only safe to call on one goroutine at
// a time.
type HistorySerializer struct {
	// A preallocated buffer to store encoded, uncompressed verdicts.
	// Avoids needing to allocate a new buffer for every decode/encode
	// operation, with consequent heap requirements and GC churn.
	tempBuf []byte
}

// ensureAndClearBuf returns a temporary buffer with suitable capacity
// and zero length.
func (hs *HistorySerializer) ensureAndClearBuf() {
	if hs.tempBuf == nil {
		// At most the history will have 2000 verdicts (cold buffer).
		// Most verdicts will be simple expected verdict, so 30,000 is probably fine
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
	buf = binary.AppendUvarint(buf, uint64(len(history.Verdicts)))

	var lastPosition uint64
	var lastHourNumber int64
	for _, verdict := range history.Verdicts {
		// We encode the relative deltaPosition between the current verdict and the
		// previous verdicts.
		deltaPosition := uint64(verdict.CommitPosition) - lastPosition
		deltaPosition = deltaPosition << 1
		if !verdict.IsSimpleExpectedPass {
			// Set the last bit to 1 if it is not a simple verdict.
			deltaPosition |= 1
		}
		buf = binary.AppendUvarint(buf, deltaPosition)
		lastPosition = uint64(verdict.CommitPosition)

		// Encode the "relative" hour.
		// Note that the relative hour may be positive or negative. So we are encoding
		// it as varint.
		hourNumber := verdict.Hour.Unix() / 3600
		deltaHour := hourNumber - lastHourNumber
		buf = binary.AppendVarint(buf, deltaHour)
		lastHourNumber = hourNumber

		// Encode the verdict details, only if not simple verdict.
		if !verdict.IsSimpleExpectedPass {
			buf = appendVerdictDetails(buf, verdict.Details)
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

// DecodeInto decodes the verdicts in buf, populating the history object.
func (hs *HistorySerializer) DecodeInto(history *History, buf []byte) error {
	// Clear existing verdicts to avoid state from a previous
	// decoding leaking.
	verdicts := history.Verdicts[:0]

	var err error
	hs.ensureAndClearBuf()
	// If it is possible hs.tempBuf was resized to be able to accept
	// all the decompressed content. If so, keep it, so we can use
	// the larger buf for future decodings.
	hs.tempBuf, err = span.Decompress(buf, hs.tempBuf)
	if err != nil {
		return errors.Annotate(err, "decompress error").Err()
	}
	reader := bytes.NewReader(hs.tempBuf)

	// Read version.
	version, err := binary.ReadUvarint(reader)
	if err != nil {
		return errors.Annotate(err, "read version").Err()
	}
	if version != EncodingVersion {
		return fmt.Errorf("version mismatched: got version %d, want %d", version, EncodingVersion)
	}

	// Read verdicts.
	nVerdicts, err := binary.ReadUvarint(reader)
	if err != nil {
		return errors.Annotate(err, "read number of verdicts").Err()
	}
	if nVerdicts > uint64(cap(verdicts)) {
		// The caller has allocated an inappropriately sized buffer.
		return errors.Reason("found %v verdicts to decode, but capacity is only %v", nVerdicts, cap(verdicts)).Err()
	}

	for i := 0; i < int(nVerdicts); i++ {
		// Get the commit position for the verdicts, and if the verdict is simple
		// expected.
		verdict := PositionVerdict{}
		posSim, err := binary.ReadUvarint(reader)
		if err != nil {
			return errors.Annotate(err, "read position simple verdict").Err()
		}
		deltaPos, isSimple := decodePositionSimpleVerdict(posSim)

		verdict.IsSimpleExpectedPass = isSimple
		// First verdict, deltaPos should be the absolute commit position.
		if i == 0 {
			verdict.CommitPosition = deltaPos
		} else {
			// deltaPos records the relative difference.
			verdict.CommitPosition = verdicts[i-1].CommitPosition + deltaPos
		}

		// Get the hour.
		deltaHour, err := binary.ReadVarint(reader)
		if err != nil {
			return errors.Annotate(err, "read delta hour").Err()
		}
		if i == 0 {
			verdict.Hour = time.Unix(deltaHour*3600, 0)
		} else {
			secs := verdicts[i-1].Hour.Unix()
			verdict.Hour = time.Unix(secs+deltaHour*3600, 0)
		}

		// Read the verdict details.
		if !isSimple {
			vd, err := readVerdictDetails(reader)
			if err != nil {
				return errors.Annotate(err, "read verdict details").Err()
			}
			verdict.Details = vd
		}
		verdicts = append(verdicts, verdict)
	}
	history.Verdicts = verdicts

	return err
}

func readVerdictDetails(reader *bytes.Reader) (VerdictDetails, error) {
	vd := VerdictDetails{}
	// Get IsExonerated.
	exoInt, err := binary.ReadUvarint(reader)
	if err != nil {
		return vd, errors.Annotate(err, "read exoneration status").Err()
	}
	vd.IsExonerated = uInt64ToBool(exoInt)

	// Get runs.
	runCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return vd, errors.Annotate(err, "read run count").Err()
	}
	vd.Runs = make([]Run, runCount)
	for i := 0; i < int(runCount); i++ {
		run, err := readRun(reader)
		if err != nil {
			return vd, errors.Annotate(err, "read run").Err()
		}
		vd.Runs[i] = run
	}
	return vd, nil
}

func readRun(reader *bytes.Reader) (Run, error) {
	r := Run{}
	// Read expected passed count.
	expectedPassedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read expected passed count").Err()
	}
	r.Expected.PassCount = int(expectedPassedCount)

	// Read expected failed count.
	expectedFailedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read expected failed count").Err()
	}
	r.Expected.FailCount = int(expectedFailedCount)

	// Read expected crashed count.
	expectedCrashedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read expected crashed count").Err()
	}
	r.Expected.CrashCount = int(expectedCrashedCount)

	// Read expected aborted count.
	expectedAbortedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read expected aborted count").Err()
	}
	r.Expected.AbortCount = int(expectedAbortedCount)

	// Read unexpected passed count.
	unexpectedPassedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read unexpected passed count").Err()
	}
	r.Unexpected.PassCount = int(unexpectedPassedCount)

	// Read unexpected failed count.
	unexpectedFailedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read unexpected failed count").Err()
	}
	r.Unexpected.FailCount = int(unexpectedFailedCount)

	// Read unexpected crashed count.
	unexpectedCrashedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read unexpected crashed count").Err()
	}
	r.Unexpected.CrashCount = int(unexpectedCrashedCount)

	// Read unexpected aborted count.
	unexpectedAbortedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read unexpected aborted count").Err()
	}
	r.Unexpected.AbortCount = int(unexpectedAbortedCount)

	// Read isDuplicate
	isDuplicate, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read is duplicate").Err()
	}
	r.IsDuplicate = uInt64ToBool(isDuplicate)

	return r, nil
}

func appendVerdictDetails(result []byte, vd VerdictDetails) []byte {
	// Encode IsExonerated.
	result = binary.AppendUvarint(result, boolToUInt64(vd.IsExonerated))

	// Encode runs.
	result = binary.AppendUvarint(result, uint64(len(vd.Runs)))
	for _, r := range vd.Runs {
		result = appendRun(result, r)
	}
	return result
}

func appendRun(result []byte, run Run) []byte {
	result = binary.AppendUvarint(result, uint64(run.Expected.PassCount))
	result = binary.AppendUvarint(result, uint64(run.Expected.FailCount))
	result = binary.AppendUvarint(result, uint64(run.Expected.CrashCount))
	result = binary.AppendUvarint(result, uint64(run.Expected.AbortCount))
	result = binary.AppendUvarint(result, uint64(run.Unexpected.PassCount))
	result = binary.AppendUvarint(result, uint64(run.Unexpected.FailCount))
	result = binary.AppendUvarint(result, uint64(run.Unexpected.CrashCount))
	result = binary.AppendUvarint(result, uint64(run.Unexpected.AbortCount))
	result = binary.AppendUvarint(result, boolToUInt64(run.IsDuplicate))
	return result
}

// decodePositionSimpleVerdict decodes the value posSim and returns 2 values.
// 1. The (delta) commit position.
// 2. Whether the verdict is a simple expected passed verdict.
// The last bit of posSim is set to 1 if the verdict is NOT a simple expected pass.
func decodePositionSimpleVerdict(posSim uint64) (int, bool) {
	isSimple := false
	lastBit := posSim & 1
	if lastBit == 0 {
		isSimple = true
	}
	deltaPos := posSim >> 1
	return int(deltaPos), isSimple
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

// compareVerdict returns 1 if v1 is later than v2, -1 if v1 is earlier than
// v2, and 0 if they are at the same time.
// The comparision is done on commit position, then on hour.
func compareVerdict(v1 PositionVerdict, v2 PositionVerdict) int {
	if v1.CommitPosition > v2.CommitPosition {
		return 1
	}
	if v1.CommitPosition < v2.CommitPosition {
		return -1
	}
	if v1.Hour.Unix() > v2.Hour.Unix() {
		return 1
	}
	if v1.Hour.Unix() < v2.Hour.Unix() {
		return -1
	}
	return 0
}
