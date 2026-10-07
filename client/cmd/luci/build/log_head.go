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
	"go.chromium.org/luci/logdog/common/types"
)

// LogHeadCmd returns the subcommand for `luci build log head`.
func LogHeadCmd(af *base.AuthFlags) *subcommands.Command {
	return newBuildLogFetchCmd(af, "head")
}

// FetchLogDogHead fetches the first headLines lines (and/or maxBytes bytes) of a LogDog stream.
func FetchLogDogHead(ctx context.Context, client logdog.LogsClient, project string, path types.StreamPath, headLines int, maxBytes int64, out io.Writer) error {
	var index int64
	first := true
	var buf bytes.Buffer

	for {
		resp, err := client.Get(ctx, &logdog.GetRequest{
			Project:       project,
			Path:          string(path),
			Index:         index,
			State:         first,
			LogCount:      64,
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

		renderLogEntries(resp.Logs, &buf)
		for _, le := range resp.Logs {
			if int64(le.StreamIndex) >= index {
				index = int64(le.StreamIndex) + 1
			} else {
				index++
			}
		}

		if headLines > 0 {
			if res, ok := artifact.ExtractHeadLines(buf.Bytes(), headLines); ok {
				if maxBytes > 0 && int64(len(res)) > maxBytes {
					res = res[:maxBytes]
				}
				_, err := out.Write(res)
				return err
			}
		} else if maxBytes > 0 && int64(buf.Len()) >= maxBytes {
			_, err := out.Write(buf.Bytes()[:maxBytes])
			return err
		}

		if len(resp.Logs) == 0 || (resp.State != nil && resp.State.TerminalIndex >= 0 && index > resp.State.TerminalIndex) {
			res := buf.Bytes()
			if headLines > 0 {
				res, _ = artifact.ExtractHeadLines(res, headLines)
			}
			if maxBytes > 0 && int64(len(res)) > maxBytes {
				res = res[:maxBytes]
			}
			_, err := out.Write(res)
			return err
		}
	}
}
