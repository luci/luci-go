// Copyright 2024 The LUCI Authors.
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

package wheels

import (
	"os"
	"strings"

	"go.chromium.org/luci/cipd/client/cipd/template"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/common"
)

// Verify the spec for all VerifyPep425Tag listed in the spec. This will ensure
// all packages existed for these platforms.
//
// TODO: Maybe implement it inside a derivation after we executing cipd binary
// directly.
func Verify(spec *vpython.Spec) error {
	for _, t := range spec.VerifyPep425Tag {
		ef, err := ensureFileFromVPythonSpec(spec, []*vpython.PEP425Tag{t})
		if err != nil {
			return err
		}
		ef.VerifyPlatforms = []template.Platform{PlatformForPEP425Tag(t)}
		var efs strings.Builder
		if err := ef.Serialize(&efs); err != nil {
			return err
		}

		cmd := common.CIPDCommand("ensure-file-verify", "-ensure-file", "-")
		cmd.Stdin = strings.NewReader(efs.String())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
