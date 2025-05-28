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

package sink

import (
	"crypto/sha256"
	"encoding/hex"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// maxOriginalSize is the maximum length of an untruncated test ID or
// test ID component.
const maxOriginalSize = 1024 * 1024 // 1MB

// truncateTestIDComponent truncates a string so that it does not
// exceed maxLengthBytes in UTF-8 when encoded into a flat test ID.
//
// If truncated is necessary, the string is truncated to
// at most (maxlengthInBytes - additionalCutoutBytes) bytes. This
// provides space for a hash or other data to be appended.
//
// Finer details:
//   - Truncation only occurs on whole unicode runes (multi-byte runes
//     will not be partially truncated) to ensure valid UTF-8. This means
//     the truncated string can be shorter than the requested size.
//   - Lengths measure the number of bytes the string occupies when
//     encoded into a flat test ID (which is in turn encoded as UTF-8),
//     not go string lengths. In particular, '\' '!' ':'
//     and '#' are taken to cost two bytes to encode rather than the
//     usual one byte because they are escaped.
//     For example, the string "hello!" ends up being encoded into a
//     flat test ID as "hello\!" and consumes 7 bytes.
//   - If truncation occurs, wasTruncated is set. This allows the caller
//     to e.g. append a hash for uniqueness.
func truncateTestIDComponent(s string, maxLengthBytes, additionalCutoutBytes int) (wasTruncated bool, result string, resultBytes int) {
	// The last index (exclusive) included in the string s truncated
	// to (maxLengthBytes - additionalCutoutBytes) bytes.
	truncationEndIndex := 0

	// The number of flat Test ID bytes in s[:truncationEndIndex].
	// This is equal to len(s[:truncationEndIndex]) plus the number of
	// characters that require escaping when encoded as a flat test ID.
	truncatedLengthInBytes := 0

	// Counts the total number of escape (\) characters required when
	// string s is encoded into a flat Test ID.
	escapesRequired := 0

	// Iterate over the string rune by rune, so we do
	// not truncate in the middle of one.
	for i, r := range s {
		// Does s[:i] fit within the target truncated size?
		if (i + escapesRequired) <= (maxLengthBytes - additionalCutoutBytes) {
			truncationEndIndex = i
			truncatedLengthInBytes = i + escapesRequired
		}
		if r == '\\' || r == '!' || r == ':' || r == '#' {
			escapesRequired++
		}
	}

	if len(s)+escapesRequired <= maxLengthBytes {
		return false, s, len(s) + escapesRequired
	}
	return true, s[:truncationEndIndex], truncatedLengthInBytes
}

// shortenIDComponent shortens the given string to not exceed maxLengthBytes
// in UTF-8 when encoded into a flat test ID.
// If it needs to be truncated, it will append a hash to ensure uniqueness.
//
// Finer details:
//   - The appended hash and separator will consume 17 bytes, so
//     17 bytes is the minimum allowed value for maxLengthBytes.
//   - Truncation only occurs on whole unicode runes (multi-byte runes
//     will not be partially truncated) to ensure valid UTF-8. This means
//     the truncated string can be shorter than the requested size.
//   - Lengths measure the number of bytes the string occupies when
//     encoded into a flat test ID (which is in turn encoded as UTF-8),
//     not go string lengths. In particular, '\' '!' ':'
//     and '#' are taken to cost two bytes to encode rather than the
//     usual one byte because they are escaped.
func shortenIDComponent(s string, maxLengthBytes int) (result string, resultBytes int, err error) {
	// Ensure the alphabet of the component we are shortening is valid, i.e.
	// is valid UTF-8, normal form C, printables only, no unicode replacement
	// character (U+FFFD).
	if err := pbutil.ValidateUTF8PrintableStrict(s, maxOriginalSize); err != nil {
		return "", 0, err
	}
	if maxLengthBytes < 17 {
		return "", 0, errors.Fmt("max length must be at least 17 bytes, got %d", maxLengthBytes)
	}

	// Truncate s to maxlengthInBytes. If it needs to be truncated, take another
	// 17 bytes off.
	isTruncated, prefix, prefixBytes := truncateTestIDComponent(s, maxLengthBytes, 17)
	if isTruncated {
		// Then append '~' and an 8-byte hash (16 hex characters).
		// With 8 bytes of hash, we should not expect collision unless
		// test results are adversarial or ~2^32 test IDs are uploaded
		// with the same prefix (see Birthday problem).
		hash := sha256.Sum256([]byte(s))
		return prefix + "~" + hex.EncodeToString(hash[:8]), prefixBytes + 1 + 16, nil
	}
	return prefix, prefixBytes, nil
}

// shortenCaseNameComponents shortens the given test case name components to
// occupy at most maxLength bytes when encoded into a flat test ID.
//
// Example: input ['a', 'b', 'c', '!'] would naturally take eight bytes in encoded
// form: "a:b:c:\!" (this includes the internal ':'s used as separators, and the
// '\' used to escape certain special characters).
//
// If the case name is truncated, a '~' followed by a 16 hexadecimal character hash
// is appended for uniqueness.
// For example, ["this", "is", "my", "very", "long", "test", "name"]
// may become   ["this", "is~01234567890abcdef"].
func shortenCaseNameComponents(components []string, maxLengthBytes int) (result []string, resultBytes int, err error) {
	if maxLengthBytes < 21 {
		return nil, 0, errors.Fmt("max length must be at least 21 bytes, got %d", maxLengthBytes)
	}
	for i, component := range components {
		// Ensure the alphabet of the component we are shortening is valid, i.e.
		// is valid UTF-8, normal form C, printables only, no unicode replacement
		// character (U+FFFD).
		if err := pbutil.ValidateUTF8PrintableStrict(component, maxOriginalSize); err != nil {
			return nil, 0, errors.Fmt("component[%d]: %w", i, err)
		}
	}

	var currentLengthBytes int
	for i, component := range components {
		if i > 0 {
			// Include the length of the ':' that is used to separate test case components
			// in the encoded flat test ID.
			currentLengthBytes += 1
		}
		// Invariant: (maxLengthBytes - currentLengthBytes) >= 21
		// (i.e. we have at least 21 bytes until our limit).

		// If we don't truncate on this component, we need to reserve at least
		// 22 bytes left for the next nested test case component, which will need:
		// - its own separating ':' (1 byte)
		// - one character of the component (up to 4 UTF-8 bytes for large runes)
		// - a '~' character (1 byte)
		// - 16 bytes for a hash.
		maxLengthForContinuation := maxLengthBytes - currentLengthBytes - (1 + 4 + 17)

		// The number of bytes we can afford to use for the text of this component.
		truncateToLength := maxLengthBytes - currentLengthBytes
		// The number of bytes extra to truncate if the text exceeds truncateToLength.
		// We need this to have enough space to append a hash.
		cutOutBytes := 17

		if (i + 1) < len(components) {
			// Non-last component. We always need at least 17 bytes remaining regardless
			// of if we truncate this component, as we may need to truncate the remaining
			// components (i+1 onwards) if end up short of bytes.
			truncateToLength -= 17
			cutOutBytes = 0
		}

		isTruncated, prefix, prefixBytes := truncateTestIDComponent(component, truncateToLength, cutOutBytes)
		if !isTruncated && prefixBytes > maxLengthForContinuation && (i+1) < len(components) {
			// Force a truncation.
			// We don't have the 22 bytes we may need for the next component.
			// While it isn't a truncation of this component's text, it is
			// a truncation of the remaining components (i+1 onwards).
			isTruncated = true
		}

		if isTruncated {
			// Append '~' and an 8-byte hash (as 16 hex characters).
			hash := sha256.Sum256([]byte(pbutil.EncodeCaseName(components...)))
			result = append(result, prefix+"~"+hex.EncodeToString(hash[:8]))
			currentLengthBytes += prefixBytes + 17
			break
		} else {
			// isTruncated is false, so prefix == component.
			result = append(result, prefix)
			currentLengthBytes += prefixBytes
		}
	}
	return result, currentLengthBytes, nil
}

// shortenStructuredID shortens the given structured test ID to maxBytes
// when encoded in a flat test ID.
//
// maxBytes incorporates the size of the following elements:
//   - The internal separator between the coarse and fine name (':'), the fine name
//     and case name ('#'), and separators inside the case name for case name
//     components (':'s).
//   - The component themselves, including any '\' characters needed to escape
//     special characters inside them.
//
// It excludes the sizes of:
//   - The module name and scheme
//   - Other internal separators (between scheme and coarse name or module name and scheme).
//
// maxBytes must be at least 100 bytes.
//
// This method returns an error if the test ID to be shortened has invalid characters
// (e.g. invalid UTF-8, non-printables).
func shortenStructuredID(id *sinkpb.TestIdentifier, maxBytes int) (*sinkpb.TestIdentifier, error) {
	if maxBytes < 100 {
		// This leaves us 25 bytes for the case name in the worst case, which
		// is close to the minimum we handle. 25 is calculated as 100 minus:
		// - 2 bytes used for separators ('#' and ':')
		// - 49 bytes for the coarse name ((100-2)/2).
		// - 24 bytes for the fine name ((100-2)*3/4 - 49).
		// Which leaves 25 bytes remaining.
		return nil, errors.Fmt("max length must be at least 100 bytes, got %d", maxBytes)
	}

	// Subtract two bytes for the internal separator between coarse and fine names (':'),
	// and fine name and case name ('#').
	availableBytes := maxBytes - 2

	// The coarse name may use up to 50% of the available bytes. We are more permissive
	// than a proportionate allocation (1/3 to each part) as we want to bias towards
	// minimising truncation in test IDs that meet length limits rather fairly truncating
	// those that exceed limits.
	//
	// It is important that truncation limits not depend on the length of later test ID
	// elements. Otherwise we might truncate the common test ID prefix differently for
	// different test cases, resulting in tests failing to group.
	coarseName, coarseNameLength, err := shortenIDComponent(id.CoarseName, availableBytes/2)
	if err != nil {
		return nil, errors.Fmt("coarse_name: %w", err)
	}

	// The coarse and fine name cumulatively may use up to 75% of the available bytes.
	fineName, fineNameLength, err := shortenIDComponent(id.FineName, (availableBytes*3/4)-coarseNameLength)
	if err != nil {
		return nil, errors.Fmt("fine_name: %w", err)
	}

	// The case name may use all remaining bytes.
	remainingBytes := availableBytes - coarseNameLength - fineNameLength
	shortenedComponents, caseNameLength, err := shortenCaseNameComponents(id.CaseNameComponents, remainingBytes)
	if err != nil {
		return nil, errors.Fmt("case_name_components: %w", err)
	}

	if caseNameLength+fineNameLength+coarseNameLength+2 > maxBytes {
		// This indicates an implementation bug. This should never happen.
		return nil, errors.Fmt("logic error: didn't reach length target, got %v bytes, want at most %v", caseNameLength+fineNameLength+coarseNameLength+2, maxBytes)
	}

	return &sinkpb.TestIdentifier{
		CoarseName:         coarseName,
		FineName:           fineName,
		CaseNameComponents: shortenedComponents,
	}, nil
}

// shortenTestID shortens the given flat test ID to maxBytes (in UTF-8).
//
// This method returns an error if the test ID to be shortened has invalid characters
// (e.g. invalid UTF-8, non-printables).
func shortenTestID(id string, maxBytes int) (string, error) {
	// golang strings are just a sequence of UTF-8 bytes, so len(id) is
	// the string's length in UTF-8.
	if len(id) < maxBytes {
		return id, nil
	}

	// Ensure the alphabet of the component we are shortening is valid, i.e.
	// is valid UTF-8, normal form C, printables only.
	if err := pbutil.ValidateUTF8Printable(id, maxOriginalSize, pbutil.ValidationModeLoose); err != nil {
		return "", err
	}

	// We need to shorten the ID.
	// Target truncation for maxBytes - 17 bytes, so we have
	// space for a '~' and a 16 hexadecimal character hash.
	targetBytes := maxBytes - 17
	endIndex := 0

	// Only truncate whole runes, not on a byte boundary in
	// the middle of one. This ensures the output remains valid UTF-8.
	for i := range id {
		if i > targetBytes {
			break
		}
		// len(id[:i]) <= targetBytes, so we can extend endIndex to i.
		endIndex = i
	}
	hash := sha256.Sum256([]byte(id))
	return id[:endIndex] + "~" + hex.EncodeToString(hash[:8]), nil
}
