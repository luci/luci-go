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
	"archive/tar"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/maruel/subcommands"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Tar is 'tar' subcommand.
func Tar(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "tar app.yaml dest-tar-file",
		ShortDesc: "Tars app files",
		LongDesc: `Tars all files of a GAE services given its YAML config
path. Honors skip_files in the yaml config.`,
		CommandRun: func() subcommands.CommandRun {
			r := &tarRun{}
			r.Init(params)
			return r
		},
	}
}

type tarRun struct {
	subcommand
}

func (r *tarRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if !r.CheckArgs(args, 2, 2) {
		return 1
	}
	return r.done(tarApp(ctx, args[0], args[1]))
}

func tarApp(ctx context.Context, appYAMLFile, tarFile string) error {
	start := clock.Now(ctx)

	cfg, err := parseServiceConfig(appYAMLFile)
	if err != nil {
		return err
	}

	skipRe := make([]*regexp.Regexp, len(cfg.SkipFiles))
	for i, sf := range cfg.SkipFiles {
		var err error
		if skipRe[i], err = regexp.Compile(sf); err != nil {
			return err
		}
	}

	out, err := os.Create(tarFile)
	if err != nil {
		return err
	}
	defer out.Close()

	w := tar.NewWriter(out)
	defer w.Close()

	var walk func(fileDir, appDir string) error
	walk = func(fileDir, appDir string) error {
		return filepath.Walk(fileDir, func(filePath string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}

			relPath, err := filepath.Rel(fileDir, filePath)
			if err != nil {
				return err
			}
			appPath := path.Join(appDir, filepath.ToSlash(relPath))

			for _, s := range skipRe {
				if s.MatchString(appPath) {
					return nil
				}
			}

			logging.Debugf(ctx, "add: %s", appPath)

			target := filePath
			targetInfo := info
			if info.Mode()&os.ModeSymlink != 0 {
				target, err = filepath.EvalSymlinks(filePath)
				if err != nil {
					return err
				}

				targetInfo, err = os.Stat(target)
				if err != nil {
					return err
				}
				if targetInfo.IsDir() {
					return walk(target, appPath)
				}
			}

			hd, err := tar.FileInfoHeader(targetInfo, "")
			if err != nil {
				return err
			}
			hd.Name = appPath
			if err := w.WriteHeader(hd); err != nil {
				return err
			}

			f, err := os.Open(target)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(w, f)
			return err
		})
	}

	if err := walk(filepath.Dir(appYAMLFile), ""); err != nil {
		return err
	}

	logging.Infof(ctx, "Tarring took %s", clock.Since(ctx, start))
	return nil
}
