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
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func TailCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	usage := "tail <target> <artifact_id>"
	desc := "Print the last N lines of an artifact"
	if parentType == ParentTypeTestResult {
		usage = "tail <target> <artifact_id> | tail <inv> <test_id> <result_id> <artifact_id>"
		desc = "Print the last N lines of a test result artifact"
	} else if parentType == ParentTypeWorkUnit {
		usage = "tail <target> <artifact_id> | tail <root_inv> <work_unit_id> <artifact_id>"
		desc = "Print the last N lines of a work unit artifact"
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc: desc + " by resource name, URL, or decomposed target IDs using optimized HTTP Range requests.\n\n" +
			"By default, prints the last 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes.",
		CommandRun: func() subcommands.CommandRun {
			r := &artifactTailRun{af: af, parentType: parentType, lines: 10}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.IntVar(&r.lines, "n", 10, "Number of lines to fetch from the end of the artifact (default 10)")
			r.Flags.IntVar(&r.lines, "lines", 10, "Alias for -n")
			r.Flags.Int64Var(&r.bytes, "c", 0, "Number of bytes to fetch from the end of the artifact")
			r.Flags.Int64Var(&r.bytes, "bytes", 0, "Alias for -c")
			r.Flags.StringVar(&r.outputFile, "o", "", "Optional file path to save the artifact content to")
			r.Flags.StringVar(&r.outputFile, "output", "", "Alias for -o")
			if parentType == ParentTypeTestResult {
				r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			}
			return r
		},
	}
}

type artifactTailRun struct {
	subcommands.CommandRunBase
	af         *base.AuthFlags
	parentType ParentType
	host       string
	lines      int
	bytes      int64
	outputFile string
	legacy     bool
}

func (r *artifactTailRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if r.lines <= 0 && r.bytes <= 0 {
		fmt.Fprintf(os.Stderr, "-n/lines or -c/bytes must be positive\n")
		return 1
	}
	ctx := cli.GetContext(a, r, env)
	return executeArtifactFetch(ctx, r.af, r.host, r.outputFile, args, r.parentType, r.legacy, func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error {
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
		statusCode, totalSize, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, br, &chunkBuf)
		if err != nil {
			return err
		}
		chunk := chunkBuf.Bytes()

		if totalSize > 0 && totalSizeBytes <= 0 {
			totalSizeBytes = totalSize
		}

		if statusCode == http.StatusOK || (br.Start < 0 && int64(len(chunk)) < chunkSize) || (br.Start == 0) || (totalSizeBytes > 0 && int64(len(chunk)) >= totalSizeBytes) {
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
