// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"go.chromium.org/luci/common/api/gerrit"
	luciflag "go.chromium.org/luci/common/flag"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/sync/parallel"

	pb "go.chromium.org/luci/buildbucket/proto"
)

type clsFlag struct {
	cls []string
}

func (f *clsFlag) Register(fs *flag.FlagSet, help string) {
	fs.Var(luciflag.StringSlice(&f.cls), "cl", help)
}

const kRequirePatchset = true

// retrieveCLs retrieves GerritChange objects from f.cls.
// Makes Gerrit RPCs if necessary, in parallel.
func (f *clsFlag) retrieveCLs(ctx context.Context, httpClient *http.Client, requirePatchset bool) ([]*pb.GerritChange, error) {
	ret := make([]*pb.GerritChange, len(f.cls))
	return ret, parallel.FanOutIn(func(work chan<- func() error) {
		for i, cl := range f.cls {
			work <- func() error {
				change, err := f.retrieveCL(ctx, cl, httpClient, requirePatchset)
				if err != nil {
					return fmt.Errorf("CL %q: %s", cl, err)
				}
				ret[i] = change
				return nil
			}
		}
	})
}

// retrieveCL retrieves a GerritChange from a string.
// Makes a Gerrit RPC if necessary.
func (f *clsFlag) retrieveCL(ctx context.Context, cl string, httpClient *http.Client, requirePatchset bool) (*pb.GerritChange, error) {
	ret, err := parseCL(cl)
	switch {
	case err != nil:
		return nil, err
	case requirePatchset && ret.Patchset == 0:
		return nil, fmt.Errorf("missing patchset number")
	case ret.Project != "" && ret.Patchset != 0:
		return ret, nil
	}

	// Fetch CL info from Gerrit.
	client, err := gerrit.NewRESTClient(httpClient, ret.Host, true)
	if err != nil {
		return nil, err
	}
	change, err := client.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  ret.Change,
		Options: []gerritpb.QueryOption{gerritpb.QueryOption_CURRENT_REVISION},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CL %d from %q: %s", ret.Change, ret.Host, err)
	}

	ret.Project = change.Project
	if ret.Patchset == 0 {
		ret.Patchset = int64(change.Revisions[change.CurrentRevision].Number)
	}
	return ret, nil
}

var regexCL = regexp.MustCompile(`((\w+-)+review\.googlesource\.com)/(#/)?c/(([^\+]+)/\+/)?(\d+)(/(\d+))?`)
var regexCLCRRev = regexp.MustCompile(`crrev\.com/([ci])/(\d+)(/(\d+))?`)

// parseCL tries to retrieve a CL info from a string.
//
// It is not strict and can consume noisy strings, e.g.
// https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go
// or incomplete strings, e.g.
// https://chromium-review.googlesource.com/c/1541677
//
// Supports crrev.com.
//
// If err is nil, returned change is guaranteed to have Host and Change.
func parseCL(s string) (*pb.GerritChange, error) {
	ret := &pb.GerritChange{}
	var change, patchSet string
	if m := regexCLCRRev.FindStringSubmatch(s); m != nil {
		ret.Host = "chromium-review.googlesource.com"
		if m[1] == "i" {
			ret.Host = "chrome-internal-review.googlesource.com"
		}
		change = m[2]
		patchSet = m[4]
	} else if m := regexCL.FindStringSubmatch(s); m != nil {
		ret.Host = m[1]
		ret.Project = m[5]
		change = m[6]
		patchSet = m[8]
	}

	if ret.Host == "" {
		return nil, fmt.Errorf("does not match r%q or r%q", regexCL, regexCLCRRev)
	}

	var err error
	ret.Change, err = strconv.ParseInt(change, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid change %q: %s", change, err)
	}

	if patchSet != "" {
		ret.Patchset, err = strconv.ParseInt(patchSet, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid patchset %q: %s", patchSet, err)
		}
	}

	return ret, nil
}
