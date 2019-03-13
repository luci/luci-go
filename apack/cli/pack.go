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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag"
)

// Pack is 'pack' subcommand.
func Pack(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "pack config [config...]",
		ShortDesc: "Packages apps given their configs",
		LongDesc: `Packages the given apps to .tar files each and uploads them to
Google Cloud Storage.
`,
		CommandRun: func() subcommands.CommandRun {
			r := &packRun{}
			r.Init(params)
			r.Flags.Var(flag.StringSlice(&r.tags), "tag", "tag for the package. Can be specified multiple times")
			return r
		},
	}
}

type packRun struct {
	subcommand
	tags []string
}

func (r *packRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	return r.done(r.run(ctx, args))
}

func (r *packRun) run(ctx context.Context, configFiles []string) error {
	for _, cfg := range configFiles {
		if err := r.packApp(ctx, cfg); err != nil {
			return err
		}
	}
	return nil
}

func (r *packRun) packApp(ctx context.Context, cfgPath string) error {
	cfg, err := readConfigFile(cfgPath)
	if err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir("", "apack-tar")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tarFile := filepath.Join(tmpDir, "app.tar")
	if err := tarApp(ctx, cfg.AppConfig, tarFile); err != nil {
		return err
	}

	client, err := newStorageClient(ctx, r.params.AuthOptions)
	if err != nil {
		return err
	}

	bucketName, baseDir, err := parseGSPath(cfg.TarStoragePath)
	if err != nil {
		return err
	}
	bucket := client.Bucket(bucketName)
	baseDir = strings.TrimSuffix(baseDir, "/")

	digestFile, err := r.uploadIfNeeded(ctx, baseDir, bucket, tarFile)
	if err != nil {
		return err
	}
	digestRelPath := strings.TrimPrefix(digestFile, baseDir+"/")

	// Write tag files.
	for _, t := range r.tags {
		tagFile := bucket.Object(path.Join(baseDir, "tag:"+t))
		logWriting(ctx, tagFile)

		w := tagFile.NewWriter(ctx)
		w.ContentType = "text/plain"
		_, err := io.WriteString(w, digestRelPath)
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil
		}
	}

	return nil
}

func (r *packRun) uploadIfNeeded(ctx context.Context, baseDir string, bucket *storage.BucketHandle, tarPath string) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(hash.Sum(nil))

	if _, err := f.Seek(0, 0); err != nil {
		return "", err
	}

	hashFile := bucket.Object(path.Join(baseDir, fmt.Sprintf("sha256:%s.tar", digest)))
	switch _, err := hashFile.Attrs(ctx); {
	case err == storage.ErrObjectNotExist:
		// proceed

	case err != nil:
		return "", err

	default:
		// exists
		return hashFile.ObjectName(), nil
	}

	logWriting(ctx, hashFile)
	w := hashFile.NewWriter(ctx)
	w.ContentEncoding = "gzip"
	defer w.Close()
	if _, err = io.Copy(w, f); err != nil {
		return "", err
	}
	return hashFile.ObjectName(), nil
}

func (r *packRun) buildIfNeeded(ctx context.Context, dir string) error {
	switch _, err := os.Stat(filepath.Join(dir, "Makefile")); {
	case os.IsNotExist(err):
		return nil // No Makefile.

	case err != nil:
		return err

	default:
		// continue
	}

	return run(ctx, dir, "make", "build")
}
