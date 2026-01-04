// Copyright 2025 The LUCI Authors.
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

package testresultsv2

import (
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Decoder provides methods to interpret TestResultsV2 table values.
// It encapsulates column decoding logic for clients (e.g. test verdicts queries)
// which need to access the underlying tables directly and cannot use the higher-level
// abstractions provided by this package.
type Decoder struct {
	decompressBuf []byte
}

// DecodeSkipReason decodes a pb.SkipReason from the SkipReason column value.
func DecodeSkipReason(skipReason spanner.NullInt64) pb.SkipReason {
	if !skipReason.Valid {
		return pb.SkipReason_SKIP_REASON_UNSPECIFIED
	}
	return pb.SkipReason(skipReason.Int64)
}

// DecompressText decompresses a Zstd-compressed string.
func (d *Decoder) DecompressText(src []byte) (string, error) {
	if len(src) == 0 {
		return "", nil
	}
	var err error
	if d.decompressBuf, err = spanutil.Decompress(src, d.decompressBuf); err != nil {
		return "", err
	}
	return string(d.decompressBuf), nil
}

// DecodeFailureReason decompresses and unmarshals src to a *pb.FailureReason.
func (d *Decoder) DecodeFailureReason(src []byte) (*pb.FailureReason, error) {
	decompressed, err := d.decompress(src)
	if err != nil {
		return nil, err
	}
	if len(decompressed) == 0 {
		return nil, nil
	}
	result := &pb.FailureReason{}
	if err := proto.Unmarshal(decompressed, result); err != nil {
		return nil, errors.Fmt("unmarshal: %w", err)
	}
	PopulateFailureReasonOutputOnlyFields(result)
	return result, nil
}

// DecodeProperties decompresses and unmarshals src to a *structpb.Struct.
func (d *Decoder) DecodeProperties(src []byte) (*structpb.Struct, error) {
	decompressed, err := d.decompress(src)
	if err != nil {
		return nil, err
	}
	if len(decompressed) == 0 {
		return nil, nil
	}
	result := &structpb.Struct{}
	if err := proto.Unmarshal(decompressed, result); err != nil {
		return nil, errors.Fmt("unmarshal: %w", err)
	}
	return result, nil
}

// DecodeSkippedReason decompresses and unmarshals src to *pb.SkippedReason.
func (d *Decoder) DecodeSkippedReason(src []byte) (*pb.SkippedReason, error) {
	decompressed, err := d.decompress(src)
	if err != nil {
		return nil, err
	}
	if len(decompressed) == 0 {
		return nil, nil
	}
	result := &pb.SkippedReason{}
	if err := proto.Unmarshal(decompressed, result); err != nil {
		return nil, errors.Fmt("unmarshal: %w", err)
	}
	return result, nil
}

// DecodeFrameworkExtensions decompresses and unmarshals src to *pb.FrameworkExtensions.
func (d *Decoder) DecodeFrameworkExtensions(src []byte) (*pb.FrameworkExtensions, error) {
	decompressed, err := d.decompress(src)
	if err != nil {
		return nil, err
	}
	if len(decompressed) == 0 {
		return nil, nil
	}
	result := &pb.FrameworkExtensions{}
	if err := proto.Unmarshal(decompressed, result); err != nil {
		return nil, errors.Fmt("unmarshal: %w", err)
	}
	return result, nil
}

// DecodeTestMetadata decompresses and unmarshalls the testMetadata, testMetadataName,
// testMetadataLocationRepo and testMetadataLocationFileName to a *pb.TestMetadata.
func (d *Decoder) DecodeTestMetadata(testMetadata []byte, testMetadataName, testMetadataLocationRepo, testMetadataLocationFileName spanner.NullString) (*pb.TestMetadata, error) {
	var result *pb.TestMetadata
	if len(testMetadata) > 0 || testMetadataName.Valid {
		result = &pb.TestMetadata{}
		if len(testMetadata) > 0 {
			decompressed, err := d.decompress(testMetadata)
			if err != nil {
				return nil, err
			}
			if len(decompressed) > 0 {
				if err := proto.Unmarshal(decompressed, result); err != nil {
					return nil, err
				}
			}
		}
		if testMetadataName.Valid {
			result.Name = testMetadataName.StringVal
		}
		if result.Location != nil {
			if testMetadataLocationRepo.Valid {
				result.Location.Repo = testMetadataLocationRepo.StringVal
			}
			if testMetadataLocationFileName.Valid {
				result.Location.FileName = testMetadataLocationFileName.StringVal
			}
		}
	}
	return result, nil
}

// decompress deecompresses src. The returned slice may be reused, so not keep
// it between calls. Unlike spanutil.Decompress, decompressing an empty slice
// is valid and produces another empty slice.
func (q *Decoder) decompress(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	// Re-assign to q.decompressBuf, in case the buffer was enlarged.
	var err error
	q.decompressBuf, err = spanutil.Decompress(src, q.decompressBuf)
	return q.decompressBuf, err
}

// ToProtoDuration creates a *durationpb.Duration from a RunDurationNanos field value.
func ToProtoDuration(runDurationNanos spanner.NullInt64) *durationpb.Duration {
	if !runDurationNanos.Valid {
		return nil
	}
	return durationpb.New(time.Duration(runDurationNanos.Int64))
}

// PopulateFailureReasonOutputOnlyFields populates output only fields
// for a normalised test result.
func PopulateFailureReasonOutputOnlyFields(fr *pb.FailureReason) {
	if len(fr.Errors) > 0 {
		// Populate PrimaryErrorMessage from Errors collection.
		fr.PrimaryErrorMessage = fr.Errors[0].Message
	} else {
		fr.PrimaryErrorMessage = ""
	}
}
