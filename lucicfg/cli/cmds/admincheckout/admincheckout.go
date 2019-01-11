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

// Package admincheckout implements 'admin-checkout' subcommand.
package admincheckout

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/api/gitiles"
	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/lucicfg/cli/base"
)

// Cmd is 'admin-checkout' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "admin-checkout CHECKOUT_PATH",
		ShortDesc: "[EXPERIMENTAL] fetches all config sets from LUCI Config service",
		Advanced:  true,
		CommandRun: func() subcommands.CommandRun {
			c := &checkoutRun{}
			c.Init(params, true, true)
			c.Flags.StringVar(&c.configSetPrefix, "config-set-prefix", "", "Prefix of config sets to fetch.")
			return c
		},
	}
}

type checkoutRun struct {
	base.Subcommand

	checkoutPath    string
	configSetPrefix string
}

type checkoutResult struct {
	ConfigSets []*config.LuciConfigConfigSet `json:"config_sets,omitempty"`
}

func (c *checkoutRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.CheckArgs(args, 1, 1) {
		return 1
	}

	var err error
	c.checkoutPath, err = filepath.Abs(args[0])
	if err != nil {
		return c.Done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	svc, err := c.ConfigService(ctx)
	if err != nil {
		return c.Done(nil, err)
	}

	return c.Done(c.run(ctx, svc))
}

func (c *checkoutRun) run(ctx context.Context, svc *config.Service) (*checkoutResult, error) {
	// We need gerrit-scoped client to read tarballs.
	client, err := c.AuthClient(ctx)
	if err != nil {
		return nil, err
	}

	// We fetch only config set metadata from LUCI Config. Actual files are
	// fetched directly through Gitiles, since gitiles preserves file permission
	// bits and probably faster than LUCI Config.
	logging.Infof(ctx, "Asking LUCI Config for a list of config sets...")
	sets, err := svc.GetConfigSets().Context(ctx).Do()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the list of config sets").Err()
	}

	// Keep only ones with the prefix.
	filtered := make([]*config.LuciConfigConfigSet, 0, len(sets.ConfigSets))
	for _, set := range sets.ConfigSets {
		if strings.HasPrefix(set.ConfigSet, c.configSetPrefix) {
			filtered = append(filtered, set)
		}
	}

	progress := progressTracker{}
	progress.start(ctx, len(filtered))

	// Don't hit Gitiles too hard, limit to N calls at once. Don't abort on a
	// first error. Errors are collected in 'progress'.
	parallel.WorkPool(16, func(tasks chan<- func() error) {
		for _, set := range filtered {
			set := set
			tasks <- func() error {
				err := fetchConfigSet(ctx, client, set, c.checkoutPath)
				progress.done(set.ConfigSet, err)
				return nil // carry on on errors
			}
		}
	})
	if err := progress.err(); err != nil {
		return nil, errors.Annotate(err, "failed to fetch some config sets from gitiles").Err()
	}

	return &checkoutResult{ConfigSets: sets.ConfigSets}, nil
}

func fetchConfigSet(ctx context.Context, client *http.Client, set *config.LuciConfigConfigSet, checkoutRoot string) (err error) {
	ctx = logging.SetField(ctx, "configSet", set.ConfigSet)

	defer func() {
		if err != nil {
			err = errors.Annotate(err, "config set %q", set.ConfigSet).Err()
		}
	}()

	// revURL is something like
	//     https://<host>/<project>/+/<hash>/<path>.
	// We need to transform it to:
	//     https://<host>/<project>/+archive/<hash>/<path>.tar.gz
	revURL := ""
	if set.Revision != nil {
		revURL = set.Revision.Url
	}
	if revURL == "" {
		return errors.Reason("no revision URL is set").Err()
	}

	// Split into project URL and the stuff within the project.
	chunks := strings.SplitN(revURL, "/+/", 2)
	if len(chunks) != 2 {
		return errors.Reason("unrecognized revision URL %q", revURL).Err()
	}
	repoURL, hashAndPath := chunks[0], chunks[1]

	// Make sure this is indeed gitiles, add '/a' there to force authenticated
	// access, since we don't want to abuse anonymous quotas.
	url, err := gitiles.NormalizeRepoURL(repoURL, true)
	if err != nil {
		return errors.Annotate(err, "unrecognized revision URL %q", revURL).Err()
	}

	// Start reading the tarball.
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/+archive/%s.tar.gz", url, hashAndPath), nil)
	if err != nil {
		return errors.Annotate(err, "failed to construct http request").Err()
	}
	logging.Debugf(ctx, "Fetching %s", req.URL)
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return errors.Annotate(err, "failed to issue GET request to gitiles").Err()
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.Reason("unexpected status code %d from gitiles", resp.StatusCode).Err()
	}

	// Create the full path to the config set on disk.
	dest, err := safeJoin(checkoutRoot, set.ConfigSet)
	if err != nil {
		return errors.Annotate(err, "bad name").Err()
	}
	if err := os.MkdirAll(dest, 0777); err != nil {
		return errors.Annotate(err, "failed to create the output dir").Err()
	}

	// Extract the tarballl there.
	if err := extractTarball(ctx, resp.Body, dest); err != nil {
		return errors.Annotate(err, "failed to extract the gitiles tarball").Err()
	}
	return nil
}

func extractTarball(ctx context.Context, r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}
		logging.Debugf(ctx, "Extracting %s", hdr.Name)

		// We are interested only in a regular files.
		fi := hdr.FileInfo()
		switch {
		case fi.Mode().IsDir():
			continue
		case !fi.Mode().IsRegular():
			logging.Warningf(ctx, "Skipping non-regular file %q", hdr.Name)
			continue
		}

		absPath, err := safeJoin(dest, hdr.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0777); err != nil {
			return err
		}

		if err := writeFile(absPath, tr, hdr.FileInfo()); err != nil {
			return err
		}
	}

	return nil
}

func writeFile(path string, body io.Reader, fi os.FileInfo) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		f.Close()
		os.Remove(path) // best effort
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path) // best effort
		return err
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

func safeJoin(base, path string) (string, error) {
	out := filepath.Join(base, filepath.FromSlash(path))
	if !strings.HasPrefix(out, base+string(filepath.Separator)) {
		return "", errors.Reason("bad file path %q", path).Err()
	}
	return out, nil
}

type progressTracker struct {
	ctx   context.Context
	total int

	m       sync.Mutex
	fetched int
	errs    []error
}

func (pt *progressTracker) start(ctx context.Context, total int) {
	logging.Infof(ctx, "About to fetch %d config sets...", total)
	pt.ctx = ctx
	pt.total = total
	pt.fetched = 0
}

func (pt *progressTracker) done(set string, err error) {
	pt.m.Lock()
	pt.fetched++
	fetched := pt.fetched
	if err != nil {
		pt.errs = append(pt.errs, err)
	}
	pt.m.Unlock()

	if err == nil {
		logging.Infof(pt.ctx, "[%d / %d] fetched %q", fetched, pt.total, set)
	} else {
		logging.Errorf(pt.ctx, "[%d / %d] failed: %s", fetched, pt.total, err)
	}
}

func (pt *progressTracker) err() error {
	pt.m.Lock()
	defer pt.m.Unlock()
	if len(pt.errs) == 0 {
		return nil
	}
	return errors.NewMultiError(pt.errs...)
}
