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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// Find CAS Client. It should have been downloaded when host installs
// CIPD packages.
func findCasClient(b *bbpb.Build) (string, error) {
	var bbagentUtilityPath string
	for path, purpose := range b.Infra.Buildbucket.Agent.Purposes {
		if purpose == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_BBAGENT_UTILITY {
			bbagentUtilityPath = path
			break
		}
	}
	if bbagentUtilityPath == "" {
		return "", errors.New("Failed to find bbagent utility packages")
	}

	casClient, err := processCmd(bbagentUtilityPath, "cas")
	if err != nil {
		return "", err
	}
	return casClient, nil
}

func generateCasCmd(casClient, outputDir string, casRef *bbpb.InputDataRef_CAS) []string {
	return []string{
		casClient,
		"download",
		"-cas-instance",
		casRef.CasInstance,
		"-digest",
		fmt.Sprintf("%s/%d", casRef.Digest.Hash, casRef.Digest.SizeBytes),
		"-dir",
		outputDir,
		"-log-level",
		"info",
	}
}

func execCasCmd(ctx context.Context, args []string) error {
	// Switch to swarming system account to download CAS inputs, it won't
	// work for non-swarming backends in the future.
	// TODO(crbug.com/1114804): Handle downloading CAS inputs within tasks
	// running on non-swarming backends.
	sysCtx, err := lucictx.SwitchLocalAccount(ctx, "system")
	if err != nil {
		return errors.Fmt("could not switch to 'system' account in LUCI_CONTEXT: %w", err)
	}
	cmd := execCommandContext(sysCtx, args[0], args[1:]...)
	logging.Infof(ctx, "Running command: %s", cmd.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return errors.WrapIf(cmd.Run(), "Failed to run cas download")
}

func downloadCasFiles(ctx context.Context, b *bbpb.Build, workDir string) error {
	var casClient string
	var err error

	inputData := b.GetInfra().GetBuildbucket().GetAgent().GetInput().GetData()
	for dir, ref := range inputData {
		casRef := ref.GetCas()
		if casRef == nil {
			continue
		}

		if casClient == "" {
			if casClient, err = findCasClient(b); err != nil {
				return errors.Fmt("download cas files: %w", err)
			}
		}

		outDir := filepath.Join(workDir, dir)
		cmdArgs := generateCasCmd(casClient, outDir, casRef)
		if err = execCasCmd(ctx, cmdArgs); err != nil {
			return errors.Fmt("download cas files: %w", err)
		}
	}

	// TODO(chanli): populate ResolvedDataRef_CAS as more things use CAS in
	// luciexe.
	return nil
}
