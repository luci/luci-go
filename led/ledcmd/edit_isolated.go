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
	"os"
	"os/exec"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/command"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

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
		return errors.WrapIf(tProg.Run(), "running transform_program")
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
		return awaitNewline(ctx)
	}
}

// EditIsolated allows you to edit the isolated (cas_input_root)
// contents of the job.Definition.
//
// This implicitly collapses all isolated sources in the job.Definition into
// a single isolated source.
// The output job.Definition always has cas_user_payload.
func EditIsolated(ctx context.Context, authOpts auth.Options, jd *job.Definition, xform IsolatedTransformer) error {
	logging.Infof(ctx, "editing isolated")

	tdir, err := os.MkdirTemp("", "led-edit-isolated")
	if err != nil {
		return errors.Fmt("failed to create tempdir: %w", err)
	}
	defer func() {
		if err = os.RemoveAll(tdir); err != nil {
			logging.Errorf(ctx, "failed to cleanup temp dir %q: %s", tdir, err)
		}
	}()

	if err := ConsolidateRbeCasSources(ctx, authOpts, jd); err != nil {
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

	casClient, err := newCASClient(ctx, authOpts, jd)
	if err != nil {
		return err
	}
	defer casClient.Close()

	if err = downloadFromCas(ctx, current, casClient, tdir); err != nil {
		return err
	}

	if err := xform(ctx, tdir); err != nil {
		return err
	}

	logging.Infof(ctx, "uploading new isolated to RBE-CAS")
	casRef, err := uploadToCas(ctx, casClient, tdir)
	if err != nil {
		return errors.Fmt("errors in uploadToCas: %w", err)
	}
	logging.Infof(ctx, "isolated upload: done")
	if jd.GetSwarming() != nil {
		jd.GetSwarming().CasUserPayload = casRef
		return nil
	}

	return jd.HighLevelEdit(func(je job.HighLevelEditor) {
		je.TaskPayloadSource("", "")
		je.CASTaskPayload(job.RecipeDirectory, casRef)
	})
}

func getCASInstance(jd *job.Definition) (string, error) {
	current, err := jd.Info().CurrentIsolated()
	if err != nil {
		return "", err
	}
	casInstance := current.GetCasInstance()
	if casInstance == "" {
		if casInstance, err = jd.CasInstance(); err != nil {
			return "", err
		}
	}
	return casInstance, nil
}

func newCASClient(ctx context.Context, authOpts auth.Options, jd *job.Definition) (*client.Client, error) {
	casInstance, err := getCASInstance(jd)
	if err != nil {
		return nil, err
	}
	return casclient.NewLegacy(ctx, casclient.AddrProd, casInstance, authOpts, false)
}

func downloadFromCas(ctx context.Context, casRef *swarmingpb.CASReference, casClient *client.Client, tdir string) error {
	if casRef.GetDigest().GetHash() == "" {
		return nil
	}
	d := digest.Digest{
		Hash: casRef.Digest.Hash,
		Size: casRef.Digest.SizeBytes,
	}
	logging.Infof(ctx, "downloading from RBE-CAS...")
	_, _, err := casClient.DownloadDirectory(ctx, d, tdir, filemetadata.NewNoopCache())
	if err != nil {
		return errors.Fmt("failed to download directory: %w", err)
	}
	return nil
}

func uploadToCas(ctx context.Context, client *client.Client, dir string) (*swarmingpb.CASReference, error) {
	is := command.InputSpec{
		Inputs: []string{"."}, // entire dir
	}
	rootDg, entries, _, err := client.ComputeMerkleTree(ctx, dir, "", "", &is, filemetadata.NewNoopCache())
	if err != nil {
		return nil, errors.Fmt("failed to compute Merkle Tree: %w", err)
	}

	_, _, err = client.UploadIfMissing(ctx, entries...)
	if err != nil {
		return nil, errors.Fmt("failed to upload items: %w", err)
	}
	return &swarmingpb.CASReference{
		CasInstance: client.InstanceName,
		Digest: &swarmingpb.Digest{
			Hash:      rootDg.Hash,
			SizeBytes: rootDg.Size,
		},
	}, nil
}
