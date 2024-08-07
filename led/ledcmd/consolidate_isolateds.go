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
	"fmt"
	"io/ioutil"
	"os"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
)

// ConsolidateRbeCasSources combines RBE-CAS inputs in slice.Properties.CasInputRoot
// and CasUserPayload for swarming tasks. For the same file, the one in CasUserPayload
// will replace the one in slice.Properties.CasInputRoot.
func ConsolidateRbeCasSources(ctx context.Context, authOpts auth.Options, jd *job.Definition) error {
	if jd.GetSwarming() == nil || (jd.GetSwarming().CasUserPayload.GetDigest().GetHash() == "") {
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
	casClient, err := casclient.NewLegacy(ctx, casclient.AddrProd, jd.GetSwarming().CasUserPayload.CasInstance, authOpts, false)
	if err != nil {
		return err
	}
	defer casClient.Close()

	for i, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
		if slc.Properties == nil {
			slc.Properties = &swarmingpb.TaskProperties{}
		}
		props := slc.Properties
		if props.CasInputRoot == nil || props.CasInputRoot.Digest.GetHash() == jd.GetSwarming().CasUserPayload.Digest.Hash {
			continue
		}

		subDir := fmt.Sprintf("%s/%d", tdir, i)
		if err := downloadFromCas(ctx, props.CasInputRoot, casClient, subDir); err != nil {
			return errors.Annotate(err, "consolidation").Err()
		}
		if err := downloadFromCas(ctx, jd.GetSwarming().CasUserPayload, casClient, subDir); err != nil {
			return errors.Annotate(err, "consolidation").Err()
		}
		casRef, err := uploadToCas(ctx, casClient, subDir)
		if err != nil {
			return errors.Annotate(err, "consolidation").Err()
		}
		props.CasInputRoot.Digest = casRef.GetDigest()
	}
	if jd.GetSwarming().CasUserPayload != nil {
		jd.GetSwarming().CasUserPayload.Digest = nil
	}
	return nil
}
