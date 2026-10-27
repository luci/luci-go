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
)

func GetCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "get [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Get a test result artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags.\n\n" +
			"Supports fetching the full artifact content or a specific byte range:\n" +
			"  - By byte range: -byte-range <start>-<end> (e.g. 0-100, 500-, -500)"
	case ParentTypeWorkUnit:
		usage = "get [-invocationid <invocation_id> -workunitid <work_unit_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Get a work unit artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags.\n\n" +
			"Supports fetching the full artifact content or a specific byte range:\n" +
			"  - By byte range: -byte-range <start>-<end> (e.g. 0-100, 500-, -500)"
	default:
		usage = "get [-invocationid <invocation_id>] [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>] [-artifactid <artifact_id>] [<target>]"
		desc = "Get a work unit or test result artifact"
		longDesc = desc + " by a positional URL/ID or explicit ID flags.\n\n" +
			"Specify -invocationid for invocation artifacts,\n" +
			"-invocationid and -workunitid for work unit artifacts,\n" +
			"or -invocationid, -testid, and -resultid for test result artifacts.\n\n" +
			"Supports fetching the full artifact content or a specific byte range:\n" +
			"  - By byte range: -byte-range <start>-<end> (e.g. 0-100, 500-, -500)"
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc:  longDesc,
		CommandRun: func() subcommands.CommandRun {
			r := &artifactGetRun{}
			r.registerFetchFlags(af, parentType)
			r.Flags.StringVar(&r.byteRangeStr, "byte-range", "", "Byte range to fetch (e.g. 0-100, 500-, -500)")
			return r
		},
	}
}

type artifactGetRun struct {
	artifactCommonRun
	byteRangeStr string
}

func (r *artifactGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	effectiveParentType, handled, exitCode := r.validateFetchFlags(args)
	if handled {
		return exitCode
	}
	byteRange, err := ParseByteRange(r.byteRangeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	ctx := cli.GetContext(a, r, env)
	return r.executeFetch(ctx, effectiveParentType, func(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error {
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
