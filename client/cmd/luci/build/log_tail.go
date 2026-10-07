// Copyright 2026 The LUCI Authors.
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

package build

import (
	"bytes"
	"context"
	"io"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// LogTailCmd returns the subcommand for `luci build log tail`.
func LogTailCmd(af *base.AuthFlags) *subcommands.Command {
	return newBuildLogFetchCmd(af, "tail")
}

// FetchLogDogTail fetches the last tailLines lines (and/or maxBytes bytes) of a LogDog stream
// by calling Tail to find the end of the stream and walking backwards in chunks if needed.
func FetchLogDogTail(ctx context.Context, client logdog.LogsClient, project string, path types.StreamPath, tailLines int, maxBytes int64, out io.Writer) error {
	tailResp, err := client.Tail(ctx, &logdog.TailRequest{
		Project: project,
		Path:    string(path),
		State:   true,
	})
	if err != nil {
		return err
	}
	if err := validateStreamDescriptor(tailResp.Desc); err != nil {
		return err
	}
	if len(tailResp.Logs) == 0 {
		return nil
	}

	var tailBuf bytes.Buffer
	renderLogEntries(tailResp.Logs, &tailBuf)
	acc := tailBuf.Bytes()
	firstTailIdx := int64(tailResp.Logs[0].StreamIndex)

	extractSatisfied := func(data []byte, isBeginning bool) ([]byte, bool) {
		if tailLines > 0 {
			res, ok := artifact.ExtractTailLines(data, tailLines, isBeginning)
			if !ok {
				return nil, false
			}
			if maxBytes > 0 && int64(len(res)) > maxBytes {
				res = res[int64(len(res))-maxBytes:]
			}
			return res, true
		}
		if maxBytes > 0 {
			if int64(len(data)) >= maxBytes {
				return data[int64(len(data))-maxBytes:], true
			}
			if isBeginning {
				return data, true
			}
			return nil, false
		}
		return data, isBeginning
	}

	if res, ok := extractSatisfied(acc, firstTailIdx == 0); ok {
		_, err := out.Write(res)
		return err
	}

	endIdx := firstTailIdx - 1
	chunkCount := int64(32)

	for endIdx >= 0 {
		startIdx := endIdx - chunkCount + 1
		if startIdx < 0 {
			startIdx = 0
		}

		var chunkEntries []*logpb.LogEntry
		curr := startIdx
		for curr <= endIdx {
			wantCount := int32(endIdx - curr + 1)
			getResp, err := client.Get(ctx, &logdog.GetRequest{
				Project:       project,
				Path:          string(path),
				Index:         curr,
				LogCount:      wantCount,
				NonContiguous: true,
			})
			if err != nil {
				return err
			}
			if len(getResp.Logs) == 0 {
				break
			}
			for _, le := range getResp.Logs {
				if int64(le.StreamIndex) <= endIdx {
					chunkEntries = append(chunkEntries, le)
				}
				if int64(le.StreamIndex) >= curr {
					curr = int64(le.StreamIndex) + 1
				} else {
					curr++
				}
			}
		}

		var chunkBuf bytes.Buffer
		renderLogEntries(chunkEntries, &chunkBuf)
		acc = append(chunkBuf.Bytes(), acc...)

		if res, ok := extractSatisfied(acc, startIdx == 0); ok {
			_, err := out.Write(res)
			return err
		}

		endIdx = startIdx - 1
		chunkCount *= 2
	}

	_, err = out.Write(acc)
	return err
}
