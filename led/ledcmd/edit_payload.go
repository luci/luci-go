// Copyright 2022 The LUCI Authors.
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

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
)

type EditPayloadOpts struct {
	// PropertyOnly determines whether to pass the recipe bundle's RBE-CAS reference
	// as a property and preserve the executable and payload of the input job
	// rather than overwriting it.
	PropertyOnly bool

	// CasDigest is the digest of the RBE-CAS reference.
	CasDigest *swarmingpb.Digest

	// CIPDPkg is the name of the CIPD package for the payload.
	CIPDPkg string
	// CIPDVer is the version of the CIPD package for the payload.
	CIPDVer string
}

// editPayloadWithCASRef overrides the job payload with given RBE-CAS reference.
func editPayloadWithCASRef(ctx context.Context, jd *job.Definition, opts *EditPayloadOpts) (err error) {
	casRef := &swarmingpb.CASReference{
		Digest: opts.CasDigest,
	}
	if casRef.CasInstance, err = getCASInstance(jd); err != nil {
		return
	}

	setRecipeBundleProperty := opts.PropertyOnly || jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties().GetFields()[job.LEDBuilderIsBootstrappedProperty].GetBoolValue()
	if setRecipeBundleProperty {
		m := &jsonpb.Marshaler{OrigName: true}
		jsonCASRef, err := m.MarshalToString(casRef)
		if err != nil {
			return errors.Annotate(err, "encoding CAS user payload").Err()
		}
		return jd.HighLevelEdit(func(je job.HighLevelEditor) {
			je.Properties(map[string]string{
				CASRecipeBundleProperty: jsonCASRef,
			}, false)
		})
	}

	if err = jd.Edit(func(je job.Editor) {
		je.ClearCurrentIsolated()
	}); err != nil {
		return err
	}
	if jd.GetSwarming() != nil {
		jd.GetSwarming().CasUserPayload = casRef
		return nil
	}
	return jd.HighLevelEdit(func(je job.HighLevelEditor) {
		je.TaskPayloadSource("", "")
		je.CASTaskPayload(job.RecipeDirectory, casRef)
	})
}

// editPayloadWithCIPD overrides the job payload with given CIPD info.
func editPayloadWithCIPD(ctx context.Context, jd *job.Definition, opts *EditPayloadOpts) (err error) {
	setRecipeBundleProperty := opts.PropertyOnly || jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties().GetFields()[job.LEDBuilderIsBootstrappedProperty].GetBoolValue()
	if setRecipeBundleProperty {
		return errors.Reason("cannot use CIPD info in property-only mode").Err()
	}

	return jd.HighLevelEdit(func(je job.HighLevelEditor) {
		pkg, ver := jd.HighLevelInfo().TaskPayloadSource()
		if opts.CIPDPkg != "" {
			pkg = opts.CIPDPkg
		}
		if opts.CIPDVer != "" {
			ver = opts.CIPDVer
		}
		je.TaskPayloadSource(pkg, ver)
	})
}

// EditPayload overrides the payload of the given job with given RBE-CAS or CIPD info.
func EditPayload(ctx context.Context, jd *job.Definition, opts *EditPayloadOpts) error {
	switch {
	case opts.CasDigest != nil:
		return editPayloadWithCASRef(ctx, jd, opts)
	case opts.CIPDPkg != "" || opts.CIPDVer != "":
		return editPayloadWithCIPD(ctx, jd, opts)
	}
	return nil
}
