// Copyright 2020 The LUCI Authors.
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

package ledcmd

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/mattn/go-tty"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/client/downloader"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/led/job"
)

// IsolatedTransformer is a function which receives a directory on the local
// disk with the contents of an isolate and is expected to manipulate the
// contents of that directory however it chooses.
//
// EditIsolated takes these functions as a callback in order to manipulate the
// isolated content of a job.Definition.
type IsolatedTransformer func(ctx context.Context, directory string) error

// ProgramIsolatedTransformer returns an IsolatedTransformer which alters the
// contents of the isolated by running a program specified with `args` in the
// directory where the isolated content has been unpacked.
func ProgramIsolatedTransformer(args ...string) IsolatedTransformer {
	return func(ctx context.Context, dir string) error {
		logging.Infof(ctx, "Invoking transform_program: %q", args)
		tProg := exec.CommandContext(ctx, args[0], args[1:]...)
		tProg.Stdout = os.Stderr
		tProg.Stderr = os.Stderr
		tProg.Dir = dir
		return errors.Annotate(tProg.Run(), "running transform_program").Err()
	}
}

// PromptIsolatedTransformer returns an IsolatedTransformer which prompts the
// user to navigate to the directory with the isolated content and manipulate
// it manually. When the user is done they should press "enter" to indicate that
// they're finished.
func PromptIsolatedTransformer() IsolatedTransformer {
	return func(ctx context.Context, dir string) error {
		logging.Infof(ctx, "")
		logging.Infof(ctx, "Edit files as you wish in:")
		logging.Infof(ctx, "\t%s", dir)

		term, err := tty.Open()
		if err != nil {
			return errors.Annotate(err, "opening terminal").Err()
		}
		defer term.Close()

		logging.Infof(ctx, "When finished, press <enter> here to isolate it.")
		_, err = term.ReadString()
		return errors.Annotate(err, "reading <enter>").Err()
	}
}

// EditIsolated allows you to edit the isolated (input ref) contents of the
// job.Definition.
//
// This implicitly collapses all isolate sources in the job.Definition into
// a single isolate.
func EditIsolated(ctx context.Context, authClient *http.Client, jd *job.Definition, xform IsolatedTransformer) error {
	logging.Infof(ctx, "editing isolated")

	tdir, err := ioutil.TempDir("", "led-edit-isolated")
	if err != nil {
		return errors.Annotate(err, "failed to create tempdir").Err()
	}
	defer func() {
		if err = os.RemoveAll(tdir); err != nil {
			logging.Errorf(ctx, "failed to cleanup temp dir %q: %s", tdir, err)
		}
	}()

	if err := ConsolidateIsolateSources(ctx, authClient, jd); err != nil {
		return err
	}

	current, err := jd.Info().CurrentIsolated()
	if err != nil {
		return err
	}
	err = jd.Edit(func(je job.Editor) {
		je.ClearCurrentIsolated()
	})
	if err != nil {
		return err
	}

	rawIsoClient := isolatedclient.New(
		nil, authClient,
		current.Server, current.Namespace,
		retry.Default,
		nil,
	)

	if current.Digest != "" {
		var statMu sync.Mutex
		var previousStats *downloader.FileStats

		dl := downloader.New(ctx, rawIsoClient, isolated.HexDigest(current.Digest), tdir, &downloader.Options{
			FileStatsCallback: func(s downloader.FileStats, span time.Duration) {
				logging.Infof(ctx, "%s", s.StatLine(previousStats, span))
				statMu.Lock()
				previousStats = &s
				statMu.Unlock()
			},
		})
		if err = dl.Wait(); err != nil {
			return err
		}
	}

	if err := xform(ctx, tdir); err != nil {
		return err
	}

	logging.Infof(ctx, "uploading new isolated")

	hash, err := isolateDirectory(ctx, rawIsoClient, tdir)
	if err != nil {
		return err
	}
	logging.Infof(ctx, "isolated upload: done")
	jd.UserPayload.Digest = string(hash)
	return nil
}

func isolateDirectory(ctx context.Context, isoClient *isolatedclient.Client, dir string) (isolated.HexDigest, error) {
	checker := archiver.NewChecker(ctx, isoClient, 32)
	uploader := archiver.NewUploader(ctx, isoClient, 8)
	arc := archiver.NewTarringArchiver(checker, uploader)

	summary, err := arc.Archive([]string{dir}, dir, nil, "", isolated.New(isoClient.Hash()))
	if err != nil {
		return "", errors.Annotate(err, "isolating directory").Err()
	}

	if err := checker.Close(); err != nil {
		return "", errors.Annotate(err, "closing checker").Err()
	}

	if err := uploader.Close(); err != nil {
		return "", errors.Annotate(err, "closing uploader").Err()
	}

	return summary.Digest, nil
}
