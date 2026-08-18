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
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func GetCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	usage := "get <target> <artifact_id>"
	desc := "Get an artifact"
	if parentType == ParentTypeTestResult {
		usage = "get <target> <artifact_id> | get <inv> <test_id> <result_id> <artifact_id>"
		desc = "Get a test result artifact"
	} else if parentType == ParentTypeWorkUnit {
		usage = "get <target> <artifact_id> | get <root_inv> <work_unit_id> <artifact_id>"
		desc = "Get a work unit artifact"
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc: desc + " by resource name, URL, or decomposed target IDs.\n\n" +
			"Supports fetching the full artifact content or a specific byte range:\n" +
			"  - By byte range: -byte-range <start>-<end> (e.g. 0-100, 500-, -500)",
		CommandRun: func() subcommands.CommandRun {
			r := &artifactGetRun{af: af, parentType: parentType}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.byteRangeStr, "byte-range", "", "Byte range to fetch (e.g. 0-100, 500-, -500)")
			r.Flags.StringVar(&r.byteRangeStr, "bytes", "", "Alias for -byte-range")
			r.Flags.StringVar(&r.outputFile, "o", "", "Optional file path to save the artifact content to")
			r.Flags.StringVar(&r.outputFile, "output", "", "Alias for -o")
			if parentType == ParentTypeTestResult {
				r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			}
			return r
		},
	}
}

type artifactGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	parentType   ParentType
	host         string
	byteRangeStr string
	outputFile   string
	legacy       bool
}

func (r *artifactGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	byteRange, err := ParseByteRange(r.byteRangeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	ctx := cli.GetContext(a, r, env)
	return executeArtifactFetch(ctx, r.af, r.host, r.outputFile, args, r.parentType, r.legacy, func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error {
		_, _, err := FetchHTTPByteRange(ctx, httpClient, fetchURL, byteRange, out)
		return err
	})
}

type ByteRange struct {
	Start int64 // -1 if suffix range (e.g. -500 means last 500 bytes)
	End   int64 // -1 if open-ended (e.g. 500- means 500 to end)
}

func ParseByteRange(s string) (*ByteRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	s = strings.ReplaceAll(s, ":", "-")
	if strings.HasPrefix(s, "-") {
		n, err := strconv.ParseInt(strings.TrimPrefix(s, "-"), 10, 64)
		if err != nil || n <= 0 {
			return nil, errors.Fmt("invalid byte range %q: must be positive length", s)
		}
		return &ByteRange{Start: -1, End: n}, nil
	}
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 1 {
		start, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 {
			return nil, errors.Fmt("invalid byte range %q", s)
		}
		return &ByteRange{Start: start, End: -1}, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 {
		return nil, errors.Fmt("invalid start byte %q in range %q", parts[0], s)
	}
	if parts[1] == "" {
		return &ByteRange{Start: start, End: -1}, nil
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return nil, errors.Fmt("invalid end byte %q in range %q: must be >= start", parts[1], s)
	}
	return &ByteRange{Start: start, End: end}, nil
}
