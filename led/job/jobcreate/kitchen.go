// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobcreate

import (
	"encoding/json"
	"flag"
	"infra/tools/kitchen/cookflags"
	"io"
	"path"

	"github.com/golang/protobuf/jsonpb"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
)

func jobDefinitionFromBuildbucketLegacy(bb *job.Buildbucket, r *swarming.SwarmingRpcsNewTaskRequest) error {
	ts := r.TaskSlices[0]

	var kitchenArgs cookflags.CookFlags
	fs := flag.NewFlagSet("kitchen_cook", flag.ContinueOnError)
	kitchenArgs.Register(fs)
	if err := fs.Parse(ts.Properties.Command[2:]); err != nil {
		return errors.Annotate(err, "parsing kitchen cook args").Err()
	}

	bbCommonFromTaskRequest(bb, r)

	bb.BbagentArgs = &bbpb.BBAgentArgs{
		CacheDir:               kitchenArgs.CacheDir,
		KnownPublicGerritHosts: ([]string)(kitchenArgs.KnownGerritHost),
		Build:                  &bbpb.Build{},

		// See note in the job.Buildbucket message for the reason we use "luciexe"
		// even in kitchen mode.
		ExecutablePath: path.Join(kitchenArgs.CheckoutDir, "luciexe"),
	}

	// kitchen builds are sorta inverted; the Build message is in the buildbucket
	// module property, but it doesn't contain the properties in input.
	const bbModPropKey = "$recipe_engine/buildbucket"
	bbModProps := kitchenArgs.Properties[bbModPropKey].(map[string]interface{})
	delete(kitchenArgs.Properties, bbModPropKey)

	pipeR, pipeW := io.Pipe()
	done := make(chan error)
	go func() {
		done <- jsonpb.Unmarshal(pipeR, bb.BbagentArgs.Build)
	}()
	if err := json.NewEncoder(pipeW).Encode(bbModProps["build"]); err != nil {
		return errors.Annotate(err, "%s['build'] -> json", bbModPropKey).Err()
	}
	if err := <-done; err != nil {
		return errors.Annotate(err, "%s['build'] -> jsonpb", bbModPropKey).Err()
	}
	bb.EnsureBasics()

	err := jsonpb.UnmarshalString(kitchenArgs.Properties.String(),
		bb.BbagentArgs.Build.Input.Properties)

	return errors.Annotate(err, "populating properties").Err()
}
