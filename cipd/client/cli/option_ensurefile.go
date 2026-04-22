// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"context"
	"flag"
	"path/filepath"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
)

////////////////////////////////////////////////////////////////////////////////
// ensureFileOptions mixin.

type legacyListFlag bool

const (
	withLegacyListFlag    legacyListFlag = true
	withoutLegacyListFlag legacyListFlag = false
)

type ensureOutFlag bool

const (
	withEnsureOutFlag    ensureOutFlag = true
	withoutEnsureOutFlag ensureOutFlag = false
)

type verifyingEnsureFile bool

const (
	requireVerifyPlatforms verifyingEnsureFile = true
	ignoreVerifyPlatforms  verifyingEnsureFile = false
)

type versionFileOpt bool

const (
	parseVersionsFile  versionFileOpt = true
	ignoreVersionsFile versionFileOpt = false
)

// ensureFileOptions defines -ensure-file flag that specifies a location of the
// "ensure file", which is a manifest that describes what should be installed
// into a site root.
type ensureFileOptions struct {
	ensureFile    string
	ensureFileOut string // used only if registerFlags got withEnsureOutFlag arg
}

func (opts *ensureFileOptions) registerFlags(f *flag.FlagSet, out ensureOutFlag, list legacyListFlag) {
	f.StringVar(&opts.ensureFile, "ensure-file", "<path>",
		`An "ensure" file. See syntax described here: `+
			`https://godoc.org/go.chromium.org/luci/cipd/client/cipd/ensure.`+
			` Providing '-' will read from stdin.`)
	if out {
		f.StringVar(&opts.ensureFileOut, "ensure-file-output", "",
			`A path to write an "ensure" file which is the fully-resolved version `+
				`of the input ensure file for the current platform. This output will `+
				`not contain any ${params} or $Settings other than $ServiceURL.`)
	}
	if list {
		f.StringVar(&opts.ensureFile, "list", "<path>", "(DEPRECATED) A synonym for -ensure-file.")
	}
}

// loadEnsureFile parses the ensure file.
func (opts *ensureFileOptions) loadEnsureFile(ctx context.Context, clientOpts *clientOptions, verifying verifyingEnsureFile, parseVers versionFileOpt) (*ensure.File, error) {
	parsedFile, err := ensure.LoadEnsureFile(opts.ensureFile)
	if err != nil {
		return nil, err
	}

	if verifying && len(parsedFile.VerifyPlatforms) == 0 {
		defaultVerifiedPlatform := template.DefaultTemplate()
		parsedFile.VerifyPlatforms = append(parsedFile.VerifyPlatforms, defaultVerifiedPlatform)

		logging.Infof(ctx, "$VerifiedPlatform directive required but not included in"+
			" ensure file, using '$VerifiedPlatform %s' as default.", defaultVerifiedPlatform)
	}

	if parseVers && parsedFile.ResolvedVersions != "" {
		clientOpts.versions, err = loadVersionsFile(parsedFile.ResolvedVersions, opts.ensureFile)
		if err != nil {
			return nil, err
		}
		logging.Debugf(ctx, "Using the resolved version file %q", filepath.Base(parsedFile.ResolvedVersions))
	}

	return parsedFile, nil
}
