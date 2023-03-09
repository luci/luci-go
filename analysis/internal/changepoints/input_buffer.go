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

package changepoints

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/common/errors"
)

const (
	// The version of the encoding to encode the verdict history.
	encodingVersion = 1
	// Capacity of the hot buffer, i.e. how many verdicts it can hold.
	defaultHotBufferCapacity = 100
	// Capacity of the cold buffer, i.e. how many verdicts it can hold.
	defaultColdBufferCapacity = 2000
)

type InputBuffer struct {
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
	// Denotes whether this verdict is a simple expected verdict or not.
	// A simple expected verdict has only one test result, which is expected.
	IsSimpleExpected bool
	// The time that this verdict is produced.
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

type Run struct {
	// Number of non-skipped expected results in the run.
	ExpectedResultCount int
	// Number of non-skipped unexpected results in the run.
	UnexpectedResultCount int
	// Whether this run is a recycled run.
	IsDuplicate bool
}

// InsertVerdict inserts a new verdict into the input buffer.
// It will first try to insert in the hot buffer, and if the hot buffer is full
// as the result of the insert, then a compaction will occur.
// If a compaction occurs, the IsColdBufferDirty flag will be set to true,
// implying that the cold buffer content needs to be written to Spanner.
func (ib *InputBuffer) InsertVerdict(v PositionVerdict) {
	// Find the position to insert the verdict.
	// As the new verdict is likely to have the the latest commit position, we
	// will iterate backwards from the end of the slice.
	verdicts := ib.HotBuffer.Verdicts
	pos := len(ib.HotBuffer.Verdicts) - 1
	for ; pos >= 0; pos-- {
		if compareVerdict(v, verdicts[pos]) == 1 {
			break
		}
	}

	verdicts = append(verdicts[:pos+1], append([]PositionVerdict{v}, verdicts[pos+1:]...)...)
	ib.HotBuffer.Verdicts = verdicts

	if len(verdicts) == ib.HotBufferCapacity {
		ib.IsColdBufferDirty = true
		ib.Compact()
	}
}

// Compact moves the content from the hot buffer to the cold buffer.
func (ib *InputBuffer) Compact() {
	// Because the hot buffer and cold buffer are both sorted, we can simply use
	// a single merge to merge the 2 buffers.
	hVerdicts := ib.HotBuffer.Verdicts
	cVerdicts := ib.ColdBuffer.Verdicts
	merged := make([]PositionVerdict, len(hVerdicts)+len(cVerdicts))

	hPos := 0
	cPos := 0
	for hPos < len(hVerdicts) && cPos < len(cVerdicts) {
		cmp := compareVerdict(hVerdicts[hPos], cVerdicts[cPos])
		// Item in hot buffer is strictly older.
		if cmp == -1 {
			merged[hPos+cPos] = hVerdicts[hPos]
			hPos++
		} else {
			merged[hPos+cPos] = cVerdicts[cPos]
			cPos++
		}
	}

	// Add the remaining items.
	for ; hPos < len(hVerdicts); hPos++ {
		merged[hPos+cPos] = hVerdicts[hPos]
	}
	for ; cPos < len(cVerdicts); cPos++ {
		merged[hPos+cPos] = cVerdicts[cPos]
	}

	ib.HotBuffer.Verdicts = []PositionVerdict{}
	// If the cold verdicts exceeds the capacity, we want to move the
	// oldest verdicts to the output buffer.
	// TODO (nqmtuan): Move extra verdicts to output buffer.
	// For now, just discard them.
	if len(merged) > ib.ColdBufferCapacity {
		merged = merged[len(merged)-ib.ColdBufferCapacity:]
	}
	ib.ColdBuffer.Verdicts = merged
}

// EncodeHistory uses varint encoding to encode history into a byte array.
// See go/luci-test-variant-analysis-design for details.
func EncodeHistory(history History) []byte {
	// At most the history will have 2000 verdicts (cold buffer).
	// Most verdicts will be simple expected verdict, so 15,000 is probably fine
	// for most cases.
	// In case 15,000 is not enough, AppendUvarint/AppendVarint will create a
	// bigger buffer to hold the values.
	result := make([]byte, 0, 15000)
	result = binary.AppendUvarint(result, uint64(encodingVersion))
	result = binary.AppendUvarint(result, uint64(len(history.Verdicts)))

	var lastPosition uint64
	var lastHourNumber int64
	for _, verdict := range history.Verdicts {
		// We encode the relative deltaPosition between the current verdict and the
		// previous verdicts.
		deltaPosition := uint64(verdict.CommitPosition) - lastPosition
		deltaPosition = deltaPosition << 1
		if !verdict.IsSimpleExpected {
			// Set the last bit to 1 if it is not a simple verdict.
			deltaPosition |= 1
		}
		result = binary.AppendUvarint(result, deltaPosition)
		lastPosition = uint64(verdict.CommitPosition)

		// Encode the "relative" hour.
		// Note that the relative hour may be positive or negative. So we are encoding
		// it as varint.
		hourNumber := verdict.Hour.Unix() / 3600
		deltaHour := hourNumber - lastHourNumber
		result = binary.AppendVarint(result, deltaHour)
		lastHourNumber = hourNumber

		// Encode the verdict details, only if not simple verdict.
		if !verdict.IsSimpleExpected {
			result = appendVerdictDetails(result, verdict.Details)
		}
	}
	// Use zstd to compress the result.
	return span.Compress(result)
}

// DecodeHistory decodes the buf and returns the history.
func DecodeHistory(buf []byte) (History, error) {
	history := History{}
	decodedBuf := make([]byte, 10000)
	decodedBuf, err := span.Decompress(buf, decodedBuf)
	if err != nil {
		return history, errors.Annotate(err, "decompress error").Err()
	}
	reader := bytes.NewReader(decodedBuf)

	// Read version.
	version, err := binary.ReadUvarint(reader)
	if err != nil {
		return history, errors.Annotate(err, "read version").Err()
	}
	if version != encodingVersion {
		return history, fmt.Errorf("version mismatched: got version %d, want %d", version, encodingVersion)
	}

	// Read verdicts.
	nVerdicts, err := binary.ReadUvarint(reader)
	if err != nil {
		return history, errors.Annotate(err, "read number of verdicts").Err()
	}
	history.Verdicts = make([]PositionVerdict, nVerdicts)
	for i := 0; i < int(nVerdicts); i++ {
		// Get the commit position for the verdicts, and if the verdict is simple
		// expected.
		verdict := PositionVerdict{}
		posSim, err := binary.ReadUvarint(reader)
		if err != nil {
			return history, errors.Annotate(err, "read position simple verdict").Err()
		}
		deltaPos, isSimple := decodePositionSimpleVerdict(posSim)

		verdict.IsSimpleExpected = isSimple
		// First verdict, deltaPos should be the absolute commit position.
		if i == 0 {
			verdict.CommitPosition = deltaPos
		} else {
			// deltaPos records the relative difference.
			verdict.CommitPosition = history.Verdicts[i-1].CommitPosition + deltaPos
		}

		// Get the hour.
		deltaHour, err := binary.ReadVarint(reader)
		if err != nil {
			return history, errors.Annotate(err, "read delta hour").Err()
		}
		if i == 0 {
			verdict.Hour = time.Unix(deltaHour*3600, 0)
		} else {
			secs := history.Verdicts[i-1].Hour.Unix()
			verdict.Hour = time.Unix(secs+deltaHour*3600, 0)
		}

		// Read the verdict details.
		if !isSimple {
			vd, err := readVerdictDetails(reader)
			if err != nil {
				return history, errors.Annotate(err, "read verdict details").Err()
			}
			verdict.Details = vd
		}
		history.Verdicts[i] = verdict
	}

	return history, err
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
	// Read expected count.
	expectedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read expected count").Err()
	}
	r.ExpectedResultCount = int(expectedCount)

	// Read unexpected count.
	unexpectedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Annotate(err, "read unexpected count").Err()
	}
	r.UnexpectedResultCount = int(unexpectedCount)

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
	result = binary.AppendUvarint(result, uint64(run.ExpectedResultCount))
	result = binary.AppendUvarint(result, uint64(run.UnexpectedResultCount))
	result = binary.AppendUvarint(result, boolToUInt64(run.IsDuplicate))
	return result
}

// decodePositionSimpleVerdict decodes the value posSim and returns 2 values.
// 1. The (delta) commit position.
// 2. Whether the verdict is a simple expected verdict.
// The last bit of posSim is set to 1 if the verdict is NOT simple expected.
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
