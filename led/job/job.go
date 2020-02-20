// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"context"
	"encoding/hex"

	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	logdog_types "go.chromium.org/luci/logdog/common/types"
)

func (jd *Definition) addLedProperties(ctx context.Context, uid string) (logdogPrefix string, err error) {
	// Set the "$recipe_engine/led" recipe properties.
	buf := make([]byte, 32)
	if _, err := cryptorand.Read(ctx, buf); err != nil {
		return "", errors.Annotate(err, "generating random token").Err()
	}
	streamName, err := logdog_types.MakeStreamName("", "led", uid, hex.EncodeToString(buf))
	if err != nil {
		return "", errors.Annotate(err, "generating logdog token").Err()
	}
	logdogPrefix = string(streamName)

	bb := jd.GetBuildbucket()
	if bb == nil {
		panic("impossible: Buildbucket is nil while flattening to swarming")
	}
	bb.EnsureBasics()

	// Pass the CIPD package or isolate containing the recipes code into
	// the led recipe module. This gives the build the information it needs
	// to launch child builds using the same version of the recipes code.
	ledProperties := map[string]interface{}{
		// The logdog prefix is unique to each led job, so it can be used as an
		// ID for the job.
		"led_run_id": string(logdogPrefix),
	}
	if payload := jd.GetUserPayload(); payload.GetDigest() != "" {
		ledProperties["isolated_input"] = map[string]interface{}{
			"server":    payload.GetServer(),
			"namespace": payload.GetNamespace(),
			"hash":      payload.GetDigest(),
		}
	} else if pkg := bb.GetBbagentArgs().GetBuild().GetExe(); pkg != nil {
		ledProperties["cipd_input"] = map[string]interface{}{
			"package": pkg.GetCipdPackage(),
			"version": pkg.GetCipdVersion(),
		}
	}

	bb.WriteProperties(map[string]interface{}{
		"$recipe_engine/led": ledProperties,
	})

	return
}

// FlattenToSwarming modifies this Definition to populate the Swarming field
// from the Buildbucket field.
//
// After flattening, buildbucket-only edit functionality will no longer work.
func (jd *Definition) FlattenToSwarming(ctx context.Context, uid string) error {
	if jd.GetSwarming() != nil {
		return nil
	}

	// Adjust bbargs to use "$recipeCheckoutDir/luciexe" as ExecutablePath.

	// TODO(iannucci): content
	// setOutputResultPath tells the task to write its result to a JSON file in the ISOLATED_OUTDIR.
	// This is an interface for getting structured output from a task launched by led.
	//func setOutputResultPath(s *Systemland) error {
	//	ka := s.KitchenArgs
	//	if ka == nil {
	//		// TODO(iannucci): Support LUCI runner.
	//		// Intentionally not fatal. led supports jobs which don't use kitchen or LUCI runner,
	//		// and we don't want to block that usage.
	//	return nil
	//	}
	//	ka.OutputResultJSONPath = "${ISOLATED_OUTDIR}/build.proto.json"
	//	return nil
	//}

	// logdogPrefix
	_, err := jd.addLedProperties(ctx, uid)
	if err != nil {
		return errors.Annotate(err, "adding led properties").Err()
	}

	// generate all dimensions

	// generate "log_location:logdog://" and "allow_milo:1" tags
	// generate "recipe_package:" and "recipe_name:" tags

	return errors.New("not implemented")
}
