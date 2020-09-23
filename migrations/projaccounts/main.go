package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"

	"go.chromium.org/luci/auth"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

type projectState struct {
	project  string
	accounts []string
}

func (s *projectState) report() {
	if len(s.accounts) == 0 {
		return
	}
	fmt.Printf("realms.project_service_accounts(\n")
	fmt.Printf("    project = %q,\n", s.project)
	fmt.Printf("    service_accounts = [\n")
	for _, acc := range s.accounts {
		fmt.Printf("        %q,\n", acc)
	}
	fmt.Printf("    ],\n")
	fmt.Printf(")\n\n")
}

func main() {
	ctx := context.Background()
	ctx = gologger.StdConfig.Use(ctx)
	if err := run(ctx); err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func withConfigClient(ctx context.Context) (context.Context, error) {
	auth := auth.NewAuthenticator(ctx, auth.SilentLogin, chromeinfra.DefaultAuthOptions())
	client, err := auth.Client()
	if err != nil {
		return nil, err
	}
	return cfgclient.Use(ctx, remote.New("luci-config.appspot.com", false, func(context.Context) (*http.Client, error) {
		return client, nil
	})), nil
}

func run(ctx context.Context) error {
	ctx, err := withConfigClient(ctx)
	if err != nil {
		return err
	}

	projects, err := cfgclient.ProjectsWithConfig(ctx, "cr-buildbucket.cfg")
	if err != nil {
		return err
	}

	var res []*projectState
	var m sync.Mutex

	err = parallel.FanOutIn(func(work chan<- func() error) {
		for _, proj := range projects {
			proj := proj
			work <- func() error {
				state, err := processProject(ctx, proj)
				if err != nil {
					logging.Errorf(ctx, "Failed when processing %s: %s", proj, err)
					return err
				}
				state.project = proj
				m.Lock()
				logging.Infof(ctx, "Done with %s...", proj)
				res = append(res, state)
				m.Unlock()
				return nil
			}
		}
	})
	if err != nil {
		return err
	}

	sort.Slice(res, func(i, j int) bool { return res[i].project < res[j].project })
	for _, s := range res {
		s.report()
	}

	return nil
}

func processProject(ctx context.Context, proj string) (*projectState, error) {
	var bb buildbucketpb.BuildbucketCfg
	var meta config.Meta

	err := cfgclient.Get(ctx,
		config.Set("projects/"+proj),
		"cr-buildbucket.cfg",
		cfgclient.ProtoText(&bb),
		&meta,
	)
	if err != nil {
		return nil, err
	}

	accounts := stringset.New(0)

	visit := func(b *buildbucketpb.Builder) {
		if b == nil {
			return
		}
		if b.ServiceAccount != "" {
			accounts.Add(b.ServiceAccount)
		}
	}

	for _, buck := range bb.Buckets {
		visit(buck.GetSwarming().GetBuilderDefaults())
		for _, b := range buck.GetSwarming().GetBuilders() {
			visit(b)
		}
	}
	for _, m := range bb.GetBuilderMixins() {
		visit(m)
	}

	return &projectState{
		accounts: accounts.ToSortedSlice(),
	}, nil
}
