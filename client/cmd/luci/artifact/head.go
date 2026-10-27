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

func HeadCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "head [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Print the first N lines of a test result artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags using optimized HTTP Range requests.\n\n" +
			"By default, prints the first 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes."
	case ParentTypeWorkUnit:
		usage = "head [-invocationid <invocation_id> -workunitid <work_unit_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Print the first N lines of a work unit artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags using optimized HTTP Range requests.\n\n" +
			"By default, prints the first 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes."
	default:
		usage = "head [-invocationid <invocation_id>] [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Print the first N lines of a work unit or test result artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags using optimized HTTP Range requests.\n\n" +
			"Specify -invocationid for invocation artifacts,\n" +
			"-invocationid and -workunitid for work unit artifacts,\n" +
			"or -invocationid, -testid, and -resultid for test result artifacts.\n\n" +
			"By default, prints the first 10 lines. You can customize the number of lines with -n / -lines, or bytes with -c / -bytes."
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc:  longDesc,
		CommandRun: func() subcommands.CommandRun {
			r := &artifactHeadRun{lines: 10}
			r.registerFetchFlags(af, parentType)
			r.Flags.IntVar(&r.lines, "n", 10, "Number of lines to fetch from the beginning of the artifact (default 10)")
			r.Flags.Int64Var(&r.bytes, "c", 0, "Number of bytes to fetch from the beginning of the artifact")
			return r
		},
	}
}

type artifactHeadRun struct {
	artifactCommonRun
	lines int
	bytes int64
}

func (r *artifactHeadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
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
		_, _, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, br, &chunkBuf)
		if err != nil {
			return err
		}
		chunk := chunkBuf.Bytes()

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
