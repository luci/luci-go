// Copyright 2024 The LUCI Authors.
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

// Package util contains utility functions.
package util

import "go.chromium.org/luci/common/errors"

// SplitToChunks splits the contents into chunks, each chunk does not exceed
// maxChunkSize number of bytes.
// Assuming the content is encoded in UTF-8.
// The function will guarantee not to split a multi-byte Unicode into
// different chunks.
// The function will attempt to split the chunk as close to maxChunkSize
// as it can, but it will also prefer splitting at line breaks ("\r\n",
// otherwise "\r" or "\n"), or whitespaces. It will scan for the last lookbackWindow bytes
// for line break/white space to split.
// If there is no linebreak or whitespace within lookbackWindow bytes,
// it will split the chunk as close to maxChunkSize (without breaking
// a multi-byte UTF-8 character).
func SplitToChunks(content []byte, maxChunkSize int, lookbackWindow int) ([]string, error) {
	if lookbackWindow > maxChunkSize {
		return nil, errors.Reason("lookback window %d must not be bigger than maxChunkSize %d", lookbackWindow, maxChunkSize).Err()
	}
	// Start index of a chunk.
	startIndex := 0
	chunks := []string{}

	// Continue chunking if the remaining content is still bigger than maxSize.
	for len(content)-startIndex > maxChunkSize {
		// Look for the byte at the end of the chunk that we can split without
		// breaking multi-byte character.
		utf8StartIndex, err := firstCharacterIndexBackward(content, startIndex+maxChunkSize)
		if err != nil {
			return nil, errors.Annotate(err, "indexOfUTF8Backward").Err()
		}
		// endIndex is the biggest index of that we can potentially split.
		endIndex := utf8StartIndex - 1

		// Look backward within lookbackWindow to find linebreak/whitespace.
		whiteSpaceIndex, whiteSpaceLength := newLineWhiteSpace(content, endIndex, lookbackWindow)
		// Found new line or white space.
		if whiteSpaceIndex != -1 {
			chunk := string(content[startIndex : whiteSpaceIndex+whiteSpaceLength])
			chunks = append(chunks, chunk)
			startIndex = whiteSpaceIndex + whiteSpaceLength
		} else { // No new line or white space, we should split at max size.
			chunk := string(content[startIndex : endIndex+1])
			chunks = append(chunks, chunk)
			startIndex = endIndex + 1
		}
	}
	// Add the last chunk.
	if startIndex < len(content) {
		chunk := string(content[startIndex:])
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// newLineWhiteSpace starts at endIndex and looks back at most
// lookbackWindow size to find a new line or white space character.
// It prioritizes in the following order:
//   - \r\n
//   - \n or \r
//   - ' ' or \t
//
// If no such character can be found, return -1.
func newLineWhiteSpace(content []byte, endIndex int, lookbackWindow int) (index int, length int) {
	nrIndex := -1
	whiteSpaceIndex := -1
	lookUntil := endIndex - lookbackWindow + 1
	if lookUntil < 0 {
		lookUntil = 0
	}
	for i := endIndex; i >= lookUntil; i-- {
		ch := content[i]
		// Check for \n\r. If we see it, return immediately.
		if ch == '\r' && i < endIndex && content[i+1] == '\n' {
			return i, 2
		}
		if ch == '\n' || ch == '\r' {
			if nrIndex == -1 {
				nrIndex = i
			}
		}
		if ch == ' ' || ch == '\t' {
			if whiteSpaceIndex == -1 {
				whiteSpaceIndex = i
			}
		}
	}
	if nrIndex != -1 {
		return nrIndex, 1
	}
	if whiteSpaceIndex != -1 {
		return whiteSpaceIndex, 1
	}
	return -1, 0
}

// firstCharacterIndexBackward looks backward from fromPosition to find
// the first index of byte that mark the start of a UTF-8 character.
func firstCharacterIndexBackward(content []byte, fromPosition int) (int, error) {
	// A UTF-8 character can take 4 bytes at most.
	toPosition := fromPosition - 3
	if toPosition < 0 {
		toPosition = 0
	}
	for i := fromPosition; i >= toPosition; i-- {
		if isUTF8StartByte(content[i]) {
			return i, nil
		}
	}
	// After 4 bytes, if we cannot find, it means the string is not in UTF-8.
	return -1, errors.New("byte slice may not be in UTF-8 format")
}

// Return true if the byte mark the start of a UTF-8 character.
// A Unicode character maybe encoded using from 1-4 bytes.
// See https://en.wikipedia.org/wiki/UTF-8
func isUTF8StartByte(b byte) bool {
	// This is an ASCII character, which only takes 1 byte.
	if b <= 0x7F {
		return true
	}
	// Multi-byte character patterns, starts with 110xxxxx, 1110xxxx, or 11110xxx.
	return b&0xC0 == 0xC0
}
