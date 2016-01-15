// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import "fmt"

// Media types.
const (
	mtGRPC         = "application/grpc"
	mtPRPC         = "application/prpc"
	mtPRPCEncoding = "encoding"
	mtPRPCBinary   = mtPRPC + "; " + mtPRPCEncoding + "=binary"
	mtPRPCJSNOPB   = mtPRPC + "; " + mtPRPCEncoding + "=json"
	mtPRPCText     = mtPRPC + "; " + mtPRPCEncoding + "=text"
	mtJSON         = "application/json"
)

type format int

const (
	formatUnspecified format = iota
	formatUnrecognized

	// the rest is ordered by preference.

	formatBinary
	formatJSONPB
	formatText
)

// parseFormat parses a format from a media type.
//
// Returns an error only if media type is invalid.
// In this case format is undefined.
func parseFormat(mediaType string, params map[string]string) (format, error) {
	switch mediaType {

	case "", "*/*", "application/*":
		return formatUnspecified, nil

	case mtGRPC:
		return formatBinary, nil

	case mtPRPC:
		for k := range params {
			if k != mtPRPCEncoding {
				return formatUnrecognized, fmt.Errorf("unexpected parameter %q", k)
			}
		}
		switch encoding := params[mtPRPCEncoding]; encoding {
		case "binary", "":
			return formatBinary, nil
		case "json":
			return formatJSONPB, nil
		case "text":
			return formatText, nil
		default:
			return formatUnrecognized, fmt.Errorf(
				`encoding parameter: invalid value %q. Valid values: "json", "binary", "text"`,
				encoding)
		}

	case mtJSON:
		return formatJSONPB, nil

	default:
		return formatUnrecognized, nil
	}
}

// acceptFormat is a format specified in "Accept" header.
type acceptFormat struct {
	Format        format
	QualityFactor float32 // preference, range: [0.0, 0.1]
}

// acceptFormatSlice is sortable by quality factor (desc) and format.
type acceptFormatSlice []acceptFormat

func (s acceptFormatSlice) Len() int {
	return len(s)
}

func (s acceptFormatSlice) Less(i, j int) bool {
	a, b := s[i], s[j]
	const epsilon = 0.000000001
	// quality factor descending
	if a.QualityFactor+epsilon > b.QualityFactor {
		return true
	}
	if a.QualityFactor+epsilon < b.QualityFactor {
		return false
	}
	// format ascending
	return a.Format < b.Format
}

func (s acceptFormatSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
