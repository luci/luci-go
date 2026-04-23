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
	"iter"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringsetflag"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
)

////////////////////////////////////////////////////////////////////////////////
// ensureFileOptions mixin.

type multiEnsureFlag int

const (
	singleEnsureFile multiEnsureFlag = iota
	zeroOrMoreEnsureFile
	oneOrMoreEnsureFile
)

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
	ensureFiles stringsetflag.Flag

	ensureFileOut string // used only if registerFlags got withEnsureOutFlag arg
}

func (opts *ensureFileOptions) registerFlags(c *cipdSubcommand, multiEnsure multiEnsureFlag, out ensureOutFlag, list legacyListFlag) {
	const efTemplate = (` See syntax described here: ` +
		`https://godoc.org/go.chromium.org/luci/cipd/client/cipd/ensure.` +
		` Providing '-' will read from stdin.`)
	var efPreamble string
	var customCheck func() (missing bool, err error)
	switch multiEnsure {
	case singleEnsureFile:
		efPreamble = `An "ensure" file.`
		if list {
			c.Flags.Var(&opts.ensureFiles, "list", "(DEPRECATED) A synonym for -ensure-file.")
		}
		customCheck = func() (missing bool, err error) {
			switch l := opts.ensureFiles.Data.Len(); {
			case l == 0:
				missing = true
			case l > 1:
				err = errors.New("ensure-file: expected exactly 1")
			}
			return
		}

	case oneOrMoreEnsureFile:
		efPreamble = `One or more "ensure" files.`
		customCheck = func() (missing bool, err error) {
			if opts.ensureFiles.Data.Len() < 1 {
				err = errors.New("ensure-file: expected at least 1")
			}
			return
		}

	case zeroOrMoreEnsureFile:
		efPreamble = `Zero or more "ensure" files.`
	}

	c.Flags.Var(&opts.ensureFiles, "ensure-file", efPreamble+efTemplate)
	if customCheck != nil {
		c.setCustomCheckFunc("ensure-file", customCheck)
	}

	if out {
		c.Flags.StringVar(&opts.ensureFileOut, "ensure-file-output", "",
			`A path to write an "ensure" file which is the fully-resolved version `+
				`of the input ensure file for the current platform. This output will `+
				`not contain any ${params} or $Settings other than $ServiceURL.`)
	}
}

type loadedEnsureFile struct {
	path string
	ef   *ensure.File
	vf   ensure.VersionsFile
}

// loadEnsureFile parses the first, and only, ensure file.
//
// Panics if no ensure files were loaded or if more than one was loaded.
//
// Should only be used when `singleEnsureFile` was passed to registerFlags.
func (opts *ensureFileOptions) loadEnsureFile(ctx context.Context, verifying verifyingEnsureFile, parseVers versionFileOpt) (*loadedEnsureFile, error) {
	next, stop := iter.Pull2(opts.allEnsureFiles(ctx, verifying, parseVers))
	defer stop()

	ef, err, ok := next()
	if !ok {
		panic("impossible: no loaded ensure files?")
	}

	if _, _, ok := next(); ok {
		panic("impossible: loaded more than one ensure file?")
	}

	return ef, err
}

// allEnsureFiles parses all the ensure files.
func (opts *ensureFileOptions) allEnsureFiles(ctx context.Context, verifying verifyingEnsureFile, parseVers versionFileOpt) iter.Seq2[*loadedEnsureFile, error] {
	// We may get multiple ensure files which refer to the same VersionsFile;
	// this map ensures that we only ever load the versions file once.
	versionFiles := map[string]ensure.VersionsFile{}

	return func(yield func(*loadedEnsureFile, error) bool) {
		for efPath := range opts.ensureFiles.Data.Iter {
			parsedFile, err := ensure.LoadEnsureFile(efPath)
			if err != nil {
				yield(nil, err)
				return
			}

			ret := &loadedEnsureFile{path: efPath, ef: parsedFile}

			if verifying && len(parsedFile.VerifyPlatforms) == 0 {
				defaultVerifiedPlatform := template.DefaultTemplate()
				parsedFile.VerifyPlatforms = append(parsedFile.VerifyPlatforms, defaultVerifiedPlatform)

				logging.Infof(ctx, "$VerifiedPlatform directive required but not included in"+
					" ensure file, using '$VerifiedPlatform %s' as default.", defaultVerifiedPlatform)
			}

			if parseVers && parsedFile.ResolvedVersions != "" {
				if cur, ok := versionFiles[parsedFile.ResolvedVersions]; ok {
					ret.vf = cur
				} else {
					ret.vf, err = loadVersionsFile(parsedFile.ResolvedVersions, efPath)
					if err != nil {
						yield(nil, err)
						return
					}
					versionFiles[parsedFile.ResolvedVersions] = ret.vf
				}
			}

			if !yield(ret, nil) {
				return
			}
		}
	}
}
