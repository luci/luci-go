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

package artifact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
)

func TailCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "tail [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Print the last N lines of a test result artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags using optimized HTTP Range requests.\n\n" +
			"By default, prints the last 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes."
	case ParentTypeWorkUnit:
		usage = "tail [-invocationid <invocation_id> -workunitid <work_unit_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Print the last N lines of a work unit artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags using optimized HTTP Range requests.\n\n" +
			"By default, prints the last 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes."
	default:
		usage = "tail [-invocationid <invocation_id>] [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Print the last N lines of a work unit or test result artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags using optimized HTTP Range requests.\n\n" +
			"Specify -invocationid for invocation artifacts,\n" +
			"-invocationid and -workunitid for work unit artifacts,\n" +
			"or -invocationid, -testid, and -resultid for test result artifacts.\n\n" +
			"By default, prints the last 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes."
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc:  longDesc,
		CommandRun: func() subcommands.CommandRun {
			r := &artifactTailRun{lines: 10}
			r.registerFetchFlags(af, parentType)
			r.Flags.IntVar(&r.lines, "n", 10, "Number of lines to fetch from the end of the artifact (default 10)")
			r.Flags.Int64Var(&r.bytes, "c", 0, "Number of bytes to fetch from the end of the artifact")
			return r
		},
	}
}

type artifactTailRun struct {
	artifactCommonRun
	lines int
	bytes int64
}

func (r *artifactTailRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	effectiveParentType, handled, exitCode := r.validateFetchFlags(args)
	if handled {
		return exitCode
	}
	if r.lines <= 0 && r.bytes <= 0 {
		fmt.Fprintf(os.Stderr, "-n/lines or -c/bytes must be positive\n")
		return 1
	}
	ctx := cli.GetContext(a, r, env)
	return r.executeFetch(ctx, effectiveParentType, func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error {
		if r.bytes > 0 {
			byteRange := &ByteRange{Start: -1, End: r.bytes}
			_, _, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, byteRange, out)
			return err
		}
		return FetchTailLines(ctx, httpClient, fetchURL, r.lines, 0, out)
	})
}

func FetchTailLines(ctx context.Context, httpClient *http.Client, fetchURL string, tailLines int, totalSizeBytes int64, out io.Writer) error {
	return FetchTailLinesWithInitialChunkSize(ctx, httpClient, fetchURL, tailLines, totalSizeBytes, 64*1024, out)
}

func FetchTailLinesWithInitialChunkSize(ctx context.Context, httpClient *http.Client, fetchURL string, tailLines int, totalSizeBytes int64, initialChunkSize int64, out io.Writer) error {
	if tailLines <= 0 {
		return nil
	}
	chunkSize := initialChunkSize
	if chunkSize <= 0 {
		chunkSize = 64 * 1024
	}

	for {
		var br *ByteRange
		isBeginning := false

		if totalSizeBytes > 0 {
			if chunkSize >= totalSizeBytes {
				isBeginning = true
				br = &ByteRange{Start: 0, End: totalSizeBytes - 1}
			} else {
				br = &ByteRange{Start: totalSizeBytes - chunkSize, End: totalSizeBytes - 1}
			}
		} else {
			br = &ByteRange{Start: -1, End: chunkSize}
		}

		var chunkBuf bytes.Buffer
		_, totalSize, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, br, &chunkBuf)
		if err != nil {
			return err
		}
		chunk := chunkBuf.Bytes()

		if totalSize > 0 && totalSizeBytes <= 0 {
			totalSizeBytes = totalSize
		}

		if (br.Start < 0 && int64(len(chunk)) < chunkSize) || (br.Start == 0) || (totalSizeBytes > 0 && int64(len(chunk)) >= totalSizeBytes) {
			isBeginning = true
		}

		res, ok := ExtractTailLines(chunk, tailLines, isBeginning)
		if ok {
			_, err = out.Write(res)
			return err
		}

		chunkSize *= 2
		if totalSizeBytes > 0 && chunkSize > totalSizeBytes {
			chunkSize = totalSizeBytes
		}
	}
}

func ExtractTailLines(data []byte, n int, isBeginningOfFile bool) ([]byte, bool) {
	if n <= 0 {
		return nil, true
	}
	if len(data) == 0 {
		return data, true
	}

	count := 0
	endIdx := len(data) - 1
	if data[endIdx] == '\n' {
		endIdx--
	}

	for i := endIdx; i >= 0; i-- {
		if data[i] == '\n' {
			count++
			if count == n {
				// Line starts immediately after this newline at i+1
				// Note: byte after '\n' is always a valid UTF-8 rune boundary.
				return data[i+1:], true
			}
		}
	}

	if isBeginningOfFile {
		return data, true
	}

	return nil, false
}
