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

// This file implements a legacy encoding, version 2.

import (
	"bytes"
	"encoding/binary"
	"time"

	"go.chromium.org/luci/common/errors"
)

const (
	// The version of the encoding to encode the verdict history.
	LegacyV2EncodingVersion = 2
)

type legacyV2VerdictDetails struct {
	// Whether a verdict is exonerated or not.
	IsExonerated bool
	// Details of the runs in the verdict.
	Runs []legacyV2Run
}

type legacyV2Run struct {
	// Counts for expected results.
	Expected ResultCounts
	// Counts for unexpected results.
	Unexpected ResultCounts
	// Whether this run is a duplicate run.
	IsDuplicate bool
}

// LegacyV2DecodeInto decodes the verdicts in buf, populating the history object.
func LegacyV2DecodeInto(history *History, reader *bytes.Reader) error {
	// Clear existing runs to avoid state from a previous
	// decoding leaking.
	runs := history.Runs[:0]

	var err error

	// Read verdicts.
	nVerdicts, err := binary.ReadUvarint(reader)
	if err != nil {
		return errors.Fmt("read number of verdicts: %w", err)
	}

	lastPosition := int64(0)
	lastHour := time.Unix(0, 0)
	for i := 0; i < int(nVerdicts); i++ {
		// Get the commit position for the verdicts, and if the verdict is simple
		// expected.
		baseRun := Run{}
		posSim, err := binary.ReadUvarint(reader)
		if err != nil {
			return errors.Fmt("read position simple verdict: %w", err)
		}
		deltaPos, isSimple := decodeLegacyV2PositionSimpleVerdict(posSim)

		// deltaPos records the relative difference.
		baseRun.SourcePosition = lastPosition + deltaPos
		lastPosition = baseRun.SourcePosition

		// Get the hour.
		deltaHour, err := binary.ReadVarint(reader)
		if err != nil {
			return errors.Fmt("read delta hour: %w", err)
		}
		baseRun.Hour = lastHour.Add(time.Duration(deltaHour) * time.Hour)
		lastHour = baseRun.Hour

		// Read the verdict details.
		if isSimple {
			run := baseRun
			run.Expected = ResultCounts{PassCount: 1}
			runs = append(runs, run)
		} else {
			vd, err := readLegacyV2VerdictDetails(reader)
			if err != nil {
				return errors.Fmt("read verdict details: %w", err)
			}
			for _, vRun := range vd.Runs {
				if vRun.IsDuplicate {
					continue
				}
				run := baseRun
				run.Expected = vRun.Expected
				run.Unexpected = vRun.Unexpected
				runs = append(runs, run)
			}
		}
	}
	history.Runs = runs

	return err
}

func readLegacyV2VerdictDetails(reader *bytes.Reader) (legacyV2VerdictDetails, error) {
	vd := legacyV2VerdictDetails{}
	// Get IsExonerated.
	exoInt, err := binary.ReadUvarint(reader)
	if err != nil {
		return vd, errors.Fmt("read exoneration status: %w", err)
	}
	vd.IsExonerated = exoInt == 1

	// Get runs.
	runCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return vd, errors.Fmt("read run count: %w", err)
	}
	vd.Runs = make([]legacyV2Run, runCount)
	for i := 0; i < int(runCount); i++ {
		run, err := readLegacyV2Run(reader)
		if err != nil {
			return vd, errors.Fmt("read run: %w", err)
		}
		vd.Runs[i] = run
	}
	return vd, nil
}

func readLegacyV2Run(reader *bytes.Reader) (legacyV2Run, error) {
	r := legacyV2Run{}
	// Read expected passed count.
	expectedPassedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read expected passed count: %w", err)
	}
	r.Expected.PassCount = int(expectedPassedCount)

	// Read expected failed count.
	expectedFailedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read expected failed count: %w", err)
	}
	r.Expected.FailCount = int(expectedFailedCount)

	// Read expected crashed count.
	expectedCrashedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read expected crashed count: %w", err)
	}
	r.Expected.CrashCount = int(expectedCrashedCount)

	// Read expected aborted count.
	expectedAbortedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read expected aborted count: %w", err)
	}
	r.Expected.AbortCount = int(expectedAbortedCount)

	// Read unexpected passed count.
	unexpectedPassedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read unexpected passed count: %w", err)
	}
	r.Unexpected.PassCount = int(unexpectedPassedCount)

	// Read unexpected failed count.
	unexpectedFailedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read unexpected failed count: %w", err)
	}
	r.Unexpected.FailCount = int(unexpectedFailedCount)

	// Read unexpected crashed count.
	unexpectedCrashedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read unexpected crashed count: %w", err)
	}
	r.Unexpected.CrashCount = int(unexpectedCrashedCount)

	// Read unexpected aborted count.
	unexpectedAbortedCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read unexpected aborted count: %w", err)
	}
	r.Unexpected.AbortCount = int(unexpectedAbortedCount)

	// Read isDuplicate
	isDuplicate, err := binary.ReadUvarint(reader)
	if err != nil {
		return r, errors.Fmt("read is duplicate: %w", err)
	}
	r.IsDuplicate = isDuplicate == 1

	return r, nil
}

// decodeLegacyV2PositionSimpleVerdict decodes the value posSim and returns 2 values.
// 1. The (delta) commit position.
// 2. Whether the verdict is a simple expected passed verdict.
// The last bit of posSim is set to 1 if the verdict is NOT a simple expected pass.
func decodeLegacyV2PositionSimpleVerdict(posSim uint64) (int64, bool) {
	isSimple := false
	lastBit := posSim & 1
	if lastBit == 0 {
		isSimple = true
	}
	deltaPos := posSim >> 1
	return int64(deltaPos), isSimple
}
