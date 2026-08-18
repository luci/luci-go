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

func HeadCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	usage := "head <target> <artifact_id>"
	desc := "Print the first N lines of an artifact"
	if parentType == ParentTypeTestResult {
		usage = "head <target> <artifact_id> | head <inv> <test_id> <result_id> <artifact_id>"
		desc = "Print the first N lines of a test result artifact"
	} else if parentType == ParentTypeWorkUnit {
		usage = "head <target> <artifact_id> | head <root_inv> <work_unit_id> <artifact_id>"
		desc = "Print the first N lines of a work unit artifact"
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc: desc + " by resource name, URL, or decomposed target IDs using optimized HTTP Range requests.\n\n" +
			"By default, prints the first 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes.",
		CommandRun: func() subcommands.CommandRun {
			r := &artifactHeadRun{af: af, parentType: parentType, lines: 10}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.IntVar(&r.lines, "n", 10, "Number of lines to fetch from the beginning of the artifact (default 10)")
			r.Flags.IntVar(&r.lines, "lines", 10, "Alias for -n")
			r.Flags.Int64Var(&r.bytes, "c", 0, "Number of bytes to fetch from the beginning of the artifact")
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

type artifactHeadRun struct {
	subcommands.CommandRunBase
	af         *base.AuthFlags
	parentType ParentType
	host       string
	lines      int
	bytes      int64
	outputFile string
	legacy     bool
}

func (r *artifactHeadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
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
			byteRange := &ByteRange{Start: 0, End: r.bytes - 1}
			_, _, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, byteRange, out)
			return err
		}
		return FetchHeadLines(ctx, httpClient, fetchURL, r.lines, out)
	})
}

func FetchHeadLines(ctx context.Context, httpClient *http.Client, fetchURL string, headLines int, out io.Writer) error {
	return FetchHeadLinesWithInitialChunkSize(ctx, httpClient, fetchURL, headLines, 64*1024, out)
}

func FetchHeadLinesWithInitialChunkSize(ctx context.Context, httpClient *http.Client, fetchURL string, headLines int, initialChunkSize int64, out io.Writer) error {
	if headLines <= 0 {
		return nil
	}
	var offset int64
	chunkSize := initialChunkSize
	if chunkSize <= 0 {
		chunkSize = 64 * 1024
	}
	var buf []byte

	for {
		var chunkBuf bytes.Buffer
		br := &ByteRange{Start: offset, End: offset + chunkSize - 1}
		statusCode, _, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, br, &chunkBuf)
		if err != nil {
			return err
		}
		chunk := chunkBuf.Bytes()

		if statusCode == http.StatusOK {
			res, _ := ExtractHeadLines(chunk, headLines)
			_, err = out.Write(res)
			return err
		}

		buf = append(buf, chunk...)
		res, ok := ExtractHeadLines(buf, headLines)
		if ok {
			_, err = out.Write(res)
			return err
		}

		if int64(len(chunk)) < chunkSize {
			// Reached EOF
			_, err = out.Write(buf)
			return err
		}

		offset += int64(len(chunk))
		chunkSize *= 2
	}
}

func ExtractHeadLines(data []byte, n int) ([]byte, bool) {
	if n <= 0 {
		return nil, true
	}
	count := 0
	for i, b := range data {
		if b == '\n' {
			count++
			if count == n {
				return data[:i+1], true
			}
		}
	}
	return data, false
}
