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
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
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
	if jd.GetSwarming() == nil {
		return nil
	}

	arc := mkIsoClient(ctx, authClient, jd.UserPayload)

	for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
		if slc.Properties == nil {
			slc.Properties = &api.TaskProperties{}
		}

		hasCasInput := slc.Properties.CasInputs != nil
		if hasCasInput {
			// extract the cmd/cwd from the isolated, if they're set.
			//
			// This is an old feature of swarming/isolated where the isolated file can
			// contain directives for the swarming task.
			cmd, cwd, err := extractCmdCwdFromIsolated(ctx, authClient, slc.Properties.CasInputs)
			if err != nil {
				return err
			}
			if len(cmd) > 0 {
				slc.Properties.Command = cmd
				slc.Properties.RelativeCwd = cwd
				// ExtraArgs is allowed to be set only if the Command is coming from the
				// isolated. However, now that we're explicitly setting the Command, we
				// must move ExtraArgs into Command.
				if len(slc.Properties.ExtraArgs) > 0 {
					slc.Properties.Command = append(slc.Properties.Command, slc.Properties.ExtraArgs...)
					slc.Properties.ExtraArgs = nil
				}
			}
		}

		if !hasCasInput && jd.UserPayload == nil {
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

func extractCmdCwdFromIsolated(ctx context.Context, authClient *http.Client, rootIso *api.CASTree) (cmd []string, cwd string, err error) {
	seenIsolateds := map[isolated.HexDigest]struct{}{}
	queue := isolated.HexDigests{isolated.HexDigest(rootIso.Digest)}
	isoClient := mkIsoClient(ctx, authClient, rootIso)

	// borrowed from go.chromium.org/luci/client/downloader.
	//
	// It's rather silly that there's no library functionality to do this.
	for len(queue) > 0 {
		iso := queue[0]
		if _, ok := seenIsolateds[iso]; ok {
			err = errors.Reason("loop detected when resolving isolate %q", rootIso).Err()
			return
		}
		seenIsolateds[iso] = struct{}{}

		var isoFile *isolated.Isolated
		if isoFile, err = fetchIsolated(ctx, isoClient, iso); err != nil {
			return
		}

		if len(isoFile.Command) > 0 {
			cmd = isoFile.Command
			cwd = isoFile.RelativeCwd
			break
		}

		queue = append(isoFile.Includes, queue[1:]...)
	}

	return
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
		if iso == nil {
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
