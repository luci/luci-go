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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/led/job"
	api "go.chromium.org/luci/swarming/proto/api"
)

// ConsolidateIsolateSources will, for Swarming tasks:
//
//   * Extract Cmd/Cwd from the slice.Properties.CasInputs (if set) and assign
//     them directly into the swarming task slice.
//   * Synthesize a new CasInput (and push it to the isolate server) which:
//     * has no includes, cwd, cmd, etc.
//     * is the union of UserPayload and slice.Properties.CasInputs
//   * Combined isolated in slice.Properties.CasInputs.
func ConsolidateIsolateSources(ctx context.Context, authClient *http.Client, jd *job.Definition) error {
	if jd.GetSwarming() == nil || jd.UserPayload == nil {
		return nil
	}

	arc := mkIsoClient(ctx, authClient, jd.UserPayload)

	for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
		if slc.Properties == nil {
			slc.Properties = &api.TaskProperties{}
		}

		if slc.Properties.CasInputs == nil && jd.UserPayload == nil {
			continue
		}

		var err error
		slc.Properties.CasInputs, err = combineIsolateds(ctx, arc,
			jd.UserPayload, slc.Properties.CasInputs)
		if err != nil {
			return errors.Annotate(err, "combining isolateds").Err()
		}
	}

	// If we 'consolidated' the UserPayload to the task slices, blank out the
	// UserPayload's digest. If there were no task slices, we want to hang onto
	// UserPayload as it's the only content (though this is a pretty pathological
	// case).
	if len(jd.GetSwarming().GetTask().GetTaskSlices()) > 0 {
		if jd.UserPayload != nil {
			jd.UserPayload.Digest = ""
		}
	}

	return nil
}

func combineIsolateds(ctx context.Context, arc isoClientIface, isos ...*api.CASTree) (*api.CASTree, error) {
	isoOut := isolated.New(isolated.GetHash(arc.Namespace()))
	digests := map[string]struct{}{}

	var processIso func(string) error
	processIso = func(dgst string) error {
		if _, ok := digests[dgst]; ok {
			return nil
		}
		digests[dgst] = struct{}{}

		isoContent, err := fetchIsolated(ctx, arc, isolated.HexDigest(dgst))
		if err != nil {
			return errors.New("fetching isolated")
		}
		for path, file := range isoContent.Files {
			if _, ok := isoOut.Files[path]; ok {
				return errors.Reason("isolated includes overlapping path %q", path).Err()
			}
			isoOut.Files[path] = file
		}
		for _, inc := range isoContent.Includes {
			if err := processIso(string(inc)); err != nil {
				return errors.Annotate(err, "processing included isolated %q", inc).Err()
			}
		}
		return nil
	}

	for _, iso := range isos {
		if iso.GetDigest() == "" {
			continue
		}

		if iso.Server != arc.Server() {
			return nil, errors.Reason(
				"cannot combine isolates from two different servers: %q vs %q",
				arc.Server(), iso.Server).Err()
		}

		if iso.Namespace != arc.Namespace() {
			return nil, errors.Reason(
				"cannot combine isolates from two different namespaces: %q vs %q",
				arc.Namespace(), iso.Namespace).Err()
		}

		if err := processIso(iso.Digest); err != nil {
			return nil, errors.Annotate(err, "processing isolated %q", iso.Digest).Err()
		}
	}

	ret := &api.CASTree{
		Server:    arc.Server(),
		Namespace: arc.Namespace(),
	}
	isolated, err := json.Marshal(isoOut)
	if err != nil {
		return nil, errors.Annotate(err, "encoding ISOLATED.json").Err()
	}
	promise := arc.Push("ISOLATED.json", isolatedclient.NewBytesSource(isolated), 0)
	promise.WaitForHashed()
	ret.Digest = string(promise.Digest())

	return ret, arc.Close()
}

// ConsolidateRbeCasSources combines RBE-CAS inputs in slice.Properties.CasInputRoot
// and CasUserPayload for swarming tasks. For the same file, the one in CasUserPayload
// will replace the one in slice.Properties.CasInputRoot.
func ConsolidateRbeCasSources(ctx context.Context, authOpts auth.Options, jd *job.Definition) error {
	if jd.GetSwarming() == nil || (jd.CasUserPayload.GetDigest().GetHash() == "") {
		return nil
	}
	logging.Infof(ctx, "consolidating RBE-CAS sources...")
	tdir, err := ioutil.TempDir("", "led-consolidate-rbe-cas")
	if err != nil {
		return errors.Annotate(err, "failed to create tempdir in consolidation step").Err()
	}
	defer func() {
		if err = os.RemoveAll(tdir); err != nil {
			logging.Errorf(ctx, "failed to cleanup temp dir %q: %s", tdir, err)
		}
	}()
	casClient, err := casclient.NewLegacy(ctx, jd.CasUserPayload.CasInstance, authOpts, false)
	if err != nil {
		return err
	}
	defer casClient.Close()

	for i, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
		if slc.Properties == nil {
			slc.Properties = &api.TaskProperties{}
		}
		props := slc.Properties
		if props.CasInputRoot == nil || props.CasInputRoot.Digest.GetHash() == jd.CasUserPayload.Digest.Hash {
			continue
		}

		subDir := fmt.Sprintf("%s/%d", tdir, i)
		if err := downloadFromCas(ctx, props.CasInputRoot, casClient, subDir); err != nil {
			return errors.Annotate(err, "consolidation").Err()
		}
		if err := downloadFromCas(ctx, jd.CasUserPayload, casClient, subDir); err != nil {
			return errors.Annotate(err, "consolidation").Err()
		}
		digest, err := uploadToCas(ctx, casClient, subDir)
		if err != nil {
			return errors.Annotate(err, "consolidation").Err()
		}
		props.CasInputRoot.Digest = &api.Digest{
			Hash:      digest.Hash,
			SizeBytes: digest.Size,
		}
	}
	if jd.CasUserPayload != nil {
		jd.CasUserPayload.Digest = nil
	}
	return nil
}
