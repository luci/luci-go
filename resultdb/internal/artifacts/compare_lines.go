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

package artifacts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"regexp"
	"unicode/utf8"

	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"

	farm "github.com/leemcloughlin/gofarmhash"
	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	maxLineLengthBytes = 100 * 1024      // 100 KiB
	maxContentBytes    = 9 * 1024 * 1024 // 9 MiB
)

var (
	errBinaryFileDetected = appstatus.Errorf(codes.InvalidArgument, "artifact appears to be a binary file (contains lines over %d KB or invalid UTF-8 sequences)", maxLineLengthBytes/1024)
	normalizePattern      = `(?:(?:(Sat|Sun|Mon|Tue|Wed|Thu|Fri|Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec|UTC|PDT|PST)[,\s]+))|(?:(?:([A-fa-f0-9]{1,4}::?){1,7}[A-fa-f0-9]{1,4}))|(?:(?:([A-fa-f0-9]{2}[:-]){5}[A-fa-f0-9]{2}))|(?:(?:https?\:\S*))|(?:(?:\/[user|usr|tmp|dev|home|run|devices|lib|root]+\/\S*))|(?:(?:chromeos\d+-row\d+-\S*|chrome-bot@S*))|(?:(?:SSID=\S*))|(?:(?:hexdump\(len=.*))|(?:(?:\w{32,}))|(?:(?:0x[A-fa-f0-9]+|[A-fa-f0-9x]{8,}|[A-fa-f0-9]{4,}[\-\_\'\]+|\:[A-fa-f0-9]{4,}))|(?:(?:(-)?[0-9]+))`
	normalizeRegex        = regexp.MustCompile(normalizePattern)
)

type pageToken struct {
	NextByteOffset int64 `json:"b"`
	NextLineNumber int32 `json:"l"`
}

// streamChunk is used to pass data or an error from the fetching goroutine
// to the processing goroutine.
type streamChunk struct {
	Data []byte
	Err  error
}

// lineProcessor is a callback function that processes a single line from the stream.
// It receives the line content, line number, and the starting byte offset of the line.
// To stop iteration early, it can return a sentinel error, such as iterator.Done.
type lineProcessor func(line []byte, lineNumber int32, byteOffset int64) error

// fetchChunks reads from the bytestream and sends data/errors to a channel.
// This function is intended to be run in a separate goroutine.
func fetchChunks(ctx context.Context, stream bytestream.ByteStream_ReadClient, chunks chan<- streamChunk) {
	defer close(chunks)
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				select {
				case <-ctx.Done():
					return
				case chunks <- streamChunk{Err: err}:
				}
			}
			return // End of stream or a real error.
		}
		select {
		case <-ctx.Done():
			return
		case chunks <- streamChunk{Data: resp.Data}:
		}
	}
}

// processStreamByLine reads from the given bytestream and calls the provided
// line processor for each line. It handles goroutine management for fetching,
// buffering, and error handling.
// If processor returns iterator.Done for a line, that line is considered not to have been processed, and the iterator.Done error will be propogated back to the caller.
func processStreamByLine(ctx context.Context, stream bytestream.ByteStream_ReadClient, startLine int32, startByte int64, processor lineProcessor) (lastProcessedLine int32, lastProcessedByte int64, err error) {
	var buffer []byte
	currentLine := startLine
	currentByte := startByte

	chunks := make(chan streamChunk, 1)
	go fetchChunks(ctx, stream, chunks)

	for chunk := range chunks {
		if chunk.Err != nil {
			return currentLine, currentByte, chunk.Err
		}
		buffer = append(buffer, chunk.Data...)

		// Process all complete lines currently in the buffer.
		for {
			newlineIdx := bytes.IndexByte(buffer, '\n')
			if newlineIdx == -1 {
				// No more complete lines, need to read more from stream.
				if len(buffer) > maxLineLengthBytes {
					return currentLine, currentByte, errBinaryFileDetected
				}
				break
			}
			if newlineIdx > maxLineLengthBytes {
				return currentLine, currentByte, errBinaryFileDetected
			}

			line := buffer[:newlineIdx]
			if err := processor(line, currentLine, currentByte); err != nil {
				return currentLine, currentByte, err // last line not processed.  Note iteration.Done will be passed back to caller.
			}

			// Consume the line (including newline) from the buffer.
			buffer = buffer[newlineIdx+1:]
			currentLine++
			currentByte += int64(len(line)) + 1
		}
	}

	// After the loop, what's left in buffer is the final line (no trailing newline).
	if len(buffer) > 0 {
		if len(buffer) > maxLineLengthBytes {
			return currentLine, currentByte, errBinaryFileDetected
		}
		if err := processor(buffer, currentLine, currentByte); err != nil {
			return currentLine, currentByte, err // final line not processed.  Note iterator.Done will be passed back to the caller.
		}
	}

	// Check that the stream actually finished, not that the context was cancelled
	return currentLine, currentByte + int64(len(buffer)), ctx.Err()
}

// ProcessPassingStream reads a passing artifact's stream to populate a set of line hashes.
func ProcessPassingStream(ctx context.Context, stream bytestream.ByteStream_ReadClient) (map[int64]struct{}, error) {
	hashes := make(map[int64]struct{})

	processor := func(line []byte, _, _ int64) error {
		h, err := hashLine(line)
		if err != nil {
			return err
		}
		hashes[h] = struct{}{}
		return nil
	}

	_, _, err := processStreamByLine(ctx, stream, 0, 0, func(line []byte, lineNumber int32, byteOffset int64) error {
		return processor(line, int64(lineNumber), byteOffset)
	})

	if err != nil {
		return nil, err
	}
	return hashes, nil
}

// ProcessFailingStream reads the failing artifact's stream and compares it to the passing hashes.
func ProcessFailingStream(ctx context.Context, stream bytestream.ByteStream_ReadClient, passingHashes map[int64]struct{}, view pb.CompareArtifactLinesRequest_View, pageSize int32, startByte int64, startLine int32) (*pb.CompareArtifactLinesResponse, error) {
	var ranges []*pb.CompareArtifactLinesResponse_FailureOnlyRange
	var currentRangeContent bytes.Buffer
	var nextPageToken string
	var totalContentBytesInResponse int64

	rangeStartLine := int32(-1)
	rangeStartByte := int64(-1)
	lastProcessedLine := startLine
	lastProcessedByte := startByte

	processor := func(line []byte, currentLine int32, currentByte int64) error {
		h, hashErr := hashLine(line)
		if hashErr != nil {
			return hashErr
		}
		_, present := passingHashes[h]

		if !present { // This is a failure-only line.
			if rangeStartLine == -1 {
				rangeStartLine = currentLine
				rangeStartByte = currentByte
			}
			if view == pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT {
				if totalContentBytesInResponse+int64(len(line))+1 > maxContentBytes {
					return iterator.Done
				}
				if currentRangeContent.Len() > 0 {
					currentRangeContent.WriteByte('\n')
					totalContentBytesInResponse += 1
				}
				currentRangeContent.Write(line)
				totalContentBytesInResponse += int64(len(line))
			}
		} else { // This is a passing line.
			if rangeStartLine != -1 { // Close the open range.
				r := &pb.CompareArtifactLinesResponse_FailureOnlyRange{
					StartLine: rangeStartLine, EndLine: currentLine,
					StartByte: rangeStartByte, EndByte: currentByte,
				}
				if view == pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT {
					r.Lines = linesFromBytes(currentRangeContent.Bytes())
				}
				ranges = append(ranges, r)
				rangeStartLine, rangeStartByte = -1, -1
				currentRangeContent.Reset()

				if int32(len(ranges)) >= pageSize {
					return iterator.Done
				}
			}
		}
		return nil
	}

	lastProcessedLine, lastProcessedByte, err := processStreamByLine(ctx, stream, startLine, startByte, processor)
	if err != nil && err != iterator.Done {
		return nil, err
	}
	pageFull := err == iterator.Done

	// If a range was still open when we finished, close it.
	if rangeStartLine != -1 && !pageFull {
		r := &pb.CompareArtifactLinesResponse_FailureOnlyRange{
			StartLine: rangeStartLine, EndLine: lastProcessedLine + 1,
			StartByte: rangeStartByte, EndByte: lastProcessedByte,
		}
		if view == pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT {
			r.Lines = linesFromBytes(currentRangeContent.Bytes())
		}
		ranges = append(ranges, r)
	}

	// Generate page token if we broke out of the loop because the page was full.
	if pageFull {
		var err error
		nextPageToken, err = encodePageToken(&pageToken{NextByteOffset: lastProcessedByte, NextLineNumber: lastProcessedLine})
		if err != nil {
			return nil, appstatus.Errorf(codes.Internal, "failed to encode next page token: %v", err)
		}
	}

	return &pb.CompareArtifactLinesResponse{FailureOnlyRanges: ranges, NextPageToken: nextPageToken}, nil
}

func hashLine(line []byte) (int64, error) {
	if !utf8.Valid(line) {
		return 0, errBinaryFileDetected
	}
	normalized := normalizeRegex.ReplaceAllString(string(line), "")
	return int64(farm.FingerPrint64([]byte(normalized))), nil
}

func linesFromBytes(b []byte) []string {
	byteLines := bytes.Split(b, []byte("\n"))
	lines := make([]string, len(byteLines))
	for i, l := range byteLines {
		lines[i] = string(l)
	}
	return lines
}

func encodePageToken(pt *pageToken) (string, error) {
	b, err := json.Marshal(pt)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
