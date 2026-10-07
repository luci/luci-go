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

	"go.chromium.org/luci/client/cmd/luci/base"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/common/types"
)

// LogGetCmd returns the subcommand for `luci build log get`.
func LogGetCmd(af *base.AuthFlags) *subcommands.Command {
	return newBuildLogFetchCmd(af, "get")
}

// FetchLogDogFull streams a LogDog stream from index 0 to completion (or up to maxBytes if > 0).
func FetchLogDogFull(ctx context.Context, client logdog.LogsClient, project string, path types.StreamPath, maxBytes int64, out io.Writer) error {
	var index int64
	first := true
	var written int64

	for {
		resp, err := client.Get(ctx, &logdog.GetRequest{
			Project:       project,
			Path:          string(path),
			Index:         index,
			State:         first,
			NonContiguous: true,
		})
		if err != nil {
			return err
		}
		if first {
			first = false
			if err := validateStreamDescriptor(resp.Desc); err != nil {
				return err
			}
		}

		if len(resp.Logs) == 0 {
			break
		}

		var chunkBuf bytes.Buffer
		renderLogEntries(resp.Logs, &chunkBuf)
		for _, le := range resp.Logs {
			if int64(le.StreamIndex) >= index {
				index = int64(le.StreamIndex) + 1
			} else {
				index++
			}
		}

		chunk := chunkBuf.Bytes()
		if maxBytes > 0 && written+int64(len(chunk)) >= maxBytes {
			remaining := maxBytes - written
			_, err := out.Write(chunk[:remaining])
			return err
		}
		if len(chunk) > 0 {
			if _, err := out.Write(chunk); err != nil {
				return err
			}
			written += int64(len(chunk))
		}

		if resp.State != nil && resp.State.TerminalIndex >= 0 && index > resp.State.TerminalIndex {
			break
		}
	}
	return nil
}
