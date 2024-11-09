// Copyright 2021 The LUCI Authors.
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
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/sync/parallel"
	lucivalidation "go.chromium.org/luci/config/validation"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
)

func cmdMatchConfig(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "match-config [flags] CFG_PATH CL1 [CL2 ...] ",
		ShortDesc: "Match given CL(s) against given config.",
		LongDesc: text.Doc(`
			With a given configuration file, validate it, and determine the configuration
			that would apply to the given CL(s).

			CFG_PATH must be the path to a generated "commit-queue.cfg" file.
			CL1, CL2, etc. must be given as URLs to Gerrit CLs e.g.:
			  "https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3198992"
			  "https://crrev.com/c/3198992"
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &matchConfigRun{}
			r.authFlags.Register(&r.Flags, p.Auth)
			return r
		},
	}
}

type matchConfigRun struct {
	subcommands.CommandRunBase
	authFlags authcli.Flags
}

func (r *matchConfigRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.validateArgs(ctx, args); err != nil {
		return r.done(badArgsTag.Apply(err))
	}

	config, err := loadAndValidateConfig(ctx, args[0])
	if err != nil {
		return r.done(err)
	}

	// cfgmatcher works with CV's storage-layer prjcfg.ConfigGroups,
	// which includes more than just the group name.
	// So, provide cfgmatcher config groups with empty hash as it doesn't matter
	// for CV CLI use case.
	prjCfgGroups := make([]*prjcfg.ConfigGroup, len(config.ConfigGroups))
	for i, cg := range config.ConfigGroups {
		prjCfgGroups[i] = &prjcfg.ConfigGroup{Content: cg, ID: prjcfg.MakeConfigGroupID("", cg.GetName())}
	}

	clURLs := args[1:]
	results := make([]matchResult, len(clURLs))
	err = parallel.FanOutIn(func(work chan<- func() error) {
		for i, clURL := range clURLs {
			i, clURL := i, clURL
			matcher := cfgmatcher.LoadMatcherFromConfigGroups(ctx, prjCfgGroups, nil)
			work <- func() error {
				results[i] = r.match(ctx, clURL, matcher)
				return nil
			}
		}
	})
	if err != nil {
		panic("impossible: workpool returned error")
	}
	errs := errors.MultiError(nil)
	for i, mr := range results {
		fmt.Printf("\n%s:\n", clURLs[i])
		fmt.Printf("  Location: Host: %s, Repo: %s, Ref: %s\n", mr.Host, mr.Repo, mr.Ref)
		if len(mr.Names) != 0 {
			fmt.Printf("  Matched: %s\n", strings.Join(mr.Names, ", "))
		}
		if mr.Error != nil {
			fmt.Printf("  Error: %s\n", mr.Error)
			errs.MaybeAdd(mr.Error)
		}
	}
	return r.done(errs.AsError())
}

type matchResult struct {
	Host, Repo, Ref string
	Names           []string
	Error           error
}

func (r *matchConfigRun) match(ctx context.Context, url string, matcher *cfgmatcher.Matcher) matchResult {
	ret := matchResult{}

	host, change, err := gerrit.FuzzyParseURL(url)
	if err != nil {
		ret.Error = err
		return ret
	}

	// We use a new client for each CL because their hosts may be different.
	client, err := r.newGerritClient(ctx, host)
	if err != nil {
		ret.Error = err
		return ret
	}

	info, err := client.GetChange(ctx, &gerritpb.GetChangeRequest{Number: change})
	if err != nil {
		ret.Error = err
		return ret
	}

	ret.Host, ret.Repo, ret.Ref = host, info.GetProject(), info.GetRef()

	ids := matcher.Match(ret.Host, ret.Repo, ret.Ref)
	for _, id := range ids {
		ret.Names = append(ret.Names, id.Name())
	}

	if len(ret.Names) == 0 {
		ret.Error = errors.Reason("the CL did not match any config groups").Err()
	}
	if len(ret.Names) > 1 {
		ret.Error = errors.Reason("the CL matched multiple config groups").Err()
	}
	return ret
}

func (r *matchConfigRun) validateArgs(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.Reason("At least 2 arguments are required").Err()
	}
	for i, arg := range args {
		if i == 0 {
			// Ensure cfg file exists.
			_, err := os.Stat(arg)
			if err != nil {
				return err
			}
		} else {
			// Ensure CL URLs are valid.
			_, _, err := gerrit.FuzzyParseURL(arg)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func loadAndValidateConfig(ctx context.Context, cfgPath string) (*cfgpb.Config, error) {
	in, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	ret := &cfgpb.Config{}
	err = prototext.Unmarshal(in, ret)
	if err != nil {
		return nil, err
	}
	vctx := &lucivalidation.Context{Context: ctx}
	if err := validation.ValidateProjectConfig(vctx, ret); err != nil {
		return nil, err
	}
	return ret, vctx.Finalize()
}

func (r *matchConfigRun) newGerritClient(ctx context.Context, host string) (gerritpb.GerritClient, error) {
	authOpts, err := r.authFlags.Options()
	if err != nil {
		return nil, err
	}
	c, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	switch {
	case err == auth.ErrLoginRequired:
		return nil, errors.New("Login required: run `luci-cv auth-login`")
	case err != nil:
		return nil, err
	}
	return gerrit.NewRESTClient(c, host, true)
}

func (r *matchConfigRun) done(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, err)
	_, badArgs := errors.TagValueIn(badArgsTag.Key, err)
	if badArgs {
		return 2
	}
	return 1
}
