// Copyright 2017 The LUCI Authors.
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

package ensure

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/exp/slices"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// File is an in-process representation of the 'ensure file' format.
type File struct {
	ServiceURL          string
	ParanoidMode        deployer.ParanoidMode
	ResolvedVersions    string
	OverrideInstallMode pkg.InstallMode

	PackagesBySubdir map[string]PackageSlice
	VerifyPlatforms  []template.Platform
}

// ParseFile parses an ensure file from the given reader. See the package docs
// for the format of this file.
//
// This file will contain unresolved template strings for package names as well
// as unpinned package versions. Use File.Resolve() to obtain resolved+pinned
// versions of these.
func ParseFile(r io.Reader) (*File, error) {
	ret := &File{PackagesBySubdir: map[string]PackageSlice{}}

	state := itemParserState{}

	// indicates that the parser is able to read $setting lines. This is flipped
	// to false on the first non-$setting statement in the file.
	settingsAllowed := true

	lineNo := 0
	makeError := func(fmtStr string, args ...any) error {
		args = append([]any{lineNo}, args...)
		return errors.Reason("failed to parse desired state (line %d): "+fmtStr, args...).Tag(cipderr.BadArgument).Err()
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineNo++

		// Remove all space
		line := strings.TrimSpace(scanner.Text())

		if len(line) == 0 || line[0] == '#' {
			// skip blank lines and comments
			continue
		}

		tok1 := line
		tok2 := ""
		if idx := strings.IndexFunc(line, unicode.IsSpace); idx == -1 {
			// only one token. This implies a second token of ""
		} else {
			tok1, tok2 = line[:idx], strings.TrimSpace(line[idx:])
		}

		switch c := tok1[0]; c {
		case '@', '$':
			if c == '$' {
				if !settingsAllowed {
					return nil, makeError("$setting found after non-$setting statements")
				}
			} else {
				settingsAllowed = false
			}

			if p := itemParsers[strings.ToLower(tok1)]; p != nil {
				if err := p(&state, ret, tok2); err != nil {
					return nil, makeError("%s", err)
				}
			} else {
				tag := map[byte]string{'@': "@directive", '$': "$setting"}[c]
				return nil, makeError("unknown %s: %q", tag, tok1)
			}

		default:
			settingsAllowed = false
			pkg := PackageDef{tok1, tok2, lineNo}
			ret.PackagesBySubdir[state.curSubdir] = append(ret.PackagesBySubdir[state.curSubdir], pkg)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Annotate(err, "failed to read the ensure file").Tag(cipderr.IO).Err()
	}

	return ret, nil
}

// Clone returns a deep copy of the File.
func (f *File) Clone() *File {
	pkgs := make(map[string]PackageSlice)
	for k, v := range f.PackagesBySubdir {
		pkgs[k] = slices.Clone(v)
	}
	plats := slices.Clone(f.VerifyPlatforms)

	newFile := *f
	newFile.PackagesBySubdir = pkgs
	newFile.VerifyPlatforms = plats
	return &newFile
}

// VersionResolver transforms a {PackageName, Version} tuple (corresponding to
// the given `def`) into a resolved pin.
//
//   - `pkg` is guaranteed to pass common.ValidatePackageName
//   - `vers` is guaranteed to pass common.ValidateInstanceVersion
//
// VersionResolver should expect to be called concurrently from multiple
// goroutines.
type VersionResolver func(pkg, vers string) (common.Pin, error)

// ResolvedFile only contains valid, fully-resolved information and is the
// result of calling File.Resolve.
type ResolvedFile struct {
	ServiceURL          string
	ParanoidMode        deployer.ParanoidMode
	OverrideInstallMode pkg.InstallMode

	PackagesBySubdir common.PinSliceBySubdir
}

// Serialize writes the ResolvedFile to an io.Writer in canonical order.
func (f *ResolvedFile) Serialize(w io.Writer) error {
	// piggyback on top of File.Serialize.
	packagesBySubdir := make(map[string]PackageSlice, len(f.PackagesBySubdir))
	for k, v := range f.PackagesBySubdir {
		slc := make(PackageSlice, len(v))
		for i, pkg := range v {
			slc[i] = PackageDef{
				PackageTemplate:   pkg.PackageName,
				UnresolvedVersion: pkg.InstanceID,
			}
		}
		packagesBySubdir[k] = slc
	}
	return (&File{
		ServiceURL:          f.ServiceURL,
		ParanoidMode:        f.ParanoidMode,
		OverrideInstallMode: f.OverrideInstallMode,
		PackagesBySubdir:    packagesBySubdir,
	}).Serialize(w)
}

// Resolve takes the current unresolved File and expands all package templates
// using the provided expander (usually template.DefaultExpander()), and also
// resolves all versions with the provided VersionResolver, calling it
// concurrently from multiple goroutines.
//
// Returns either a single error (if something is wrong with the ensure file),
// or a multi-error with all resolution errors, sorted by definition line
// numbers.
func (f *File) Resolve(rslv VersionResolver, expander template.Expander) (*ResolvedFile, error) {
	ret := &ResolvedFile{OverrideInstallMode: f.OverrideInstallMode}

	if f.ServiceURL != "" {
		// double check the url
		if _, err := url.Parse(f.ServiceURL); err != nil {
			return nil, errors.Annotate(err, "bad ServiceURL").Tag(cipderr.BadArgument).Err()
		}
	}

	ret.ParanoidMode = deployer.NotParanoid
	if f.ParanoidMode != "" {
		if err := f.ParanoidMode.Validate(); err != nil {
			return nil, err
		}
		ret.ParanoidMode = f.ParanoidMode
	}

	ret.ServiceURL = f.ServiceURL
	if len(f.PackagesBySubdir) == 0 {
		return ret, nil
	}

	type resolveWorkItem struct {
		idx    int        // index in the definition, to preserve the ordering
		subdir string     // expanded
		pkg    string     // expanded
		def    PackageDef // original

		pin common.Pin // resolved, pin.PackageName == pkg
		err error      // resolution error
	}

	subdirs := make([]string, 0, len(f.PackagesBySubdir))
	for s := range f.PackagesBySubdir {
		subdirs = append(subdirs, s)
	}
	sort.Strings(subdirs)

	// Collect a list of package defs we want to resolve, expanding the templates
	// right away. Enumerate the map in deterministic order to make errors
	// ordered.
	var toResolve []resolveWorkItem
	for _, subdir := range subdirs {
		realSubdir, err := expander.Expand(subdir)
		switch err {
		case template.ErrSkipTemplate:
			continue
		case nil:
		default:
			return nil, errors.Annotate(err, "normalizing %q", subdir).Err()
		}

		// double-check the subdir
		if err := common.ValidateSubdir(realSubdir); err != nil {
			return nil, errors.Annotate(err, "normalizing %q", subdir).Err()
		}

		for _, def := range f.PackagesBySubdir[subdir] {
			switch realPkg, err := def.Expand(expander); {
			case err == template.ErrSkipTemplate:
				continue
			case err != nil:
				return nil, err // the error is already properly annotated
			default:
				toResolve = append(toResolve, resolveWorkItem{
					idx:    len(toResolve),
					subdir: realSubdir,
					pkg:    realPkg,
					def:    def,
				})
			}
		}
	}

	// Resolve versions into instance IDs in parallel. Errors are collected
	// through 'resolved'.
	resolved := make([]*resolveWorkItem, len(toResolve))
	parallel.FanOutIn(func(tasks chan<- func() error) {
		for _, p := range toResolve {
			tasks <- func() error {
				p.pin, p.err = rslv(p.pkg, p.def.UnresolvedVersion)
				if p.err == nil {
					p.err = common.ValidatePin(p.pin, common.AnyHash)
				}
				switch {
				case p.err != nil:
					p.err = errors.Annotate(p.err, "failed to resolve %s@%s (line %d)",
						p.pkg, p.def.UnresolvedVersion, p.def.LineNo).Err()
				case p.pin.PackageName != p.pkg:
					panic(fmt.Sprintf("bad resolver, returned wrong package name %q, expecting %q", p.pin.PackageName, p.pkg))
				}
				resolved[p.idx] = &p
				return nil
			}
		}
	})

	// subdir -> pkg -> orig_lineno
	resolvedPkgDupList := map[string]map[string]int{}

	// Check and split the result.
	ret.PackagesBySubdir = common.PinSliceBySubdir{}
	var merr errors.MultiError
	for _, p := range resolved {
		if p.err != nil {
			merr = append(merr, p.err)
			continue
		}

		if origLineNo, ok := resolvedPkgDupList[p.subdir][p.pkg]; ok {
			merr = append(merr, errors.
				Reason("duplicate package in subdir %q: %q: defined on line %d and %d",
					p.subdir, p.pkg, origLineNo, p.def.LineNo).Tag(cipderr.BadArgument).Err())
			continue
		}

		if resolvedPkgDupList[p.subdir] == nil {
			resolvedPkgDupList[p.subdir] = map[string]int{}
		}
		resolvedPkgDupList[p.subdir][p.pkg] = p.def.LineNo

		ret.PackagesBySubdir[p.subdir] = append(ret.PackagesBySubdir[p.subdir], p.pin)
	}

	if len(merr) != 0 {
		return nil, merr
	}
	return ret, nil
}

// Serialize writes the File to an io.Writer in canonical order.
func (f *File) Serialize(w io.Writer) error {
	_, err := iotools.WriteTracker(w, func(w io.Writer) error {
		needsNLs := 0
		maybeAddNL := func() {
			if needsNLs > 0 {
				w.Write(bytes.Repeat([]byte("\n"), needsNLs))
				needsNLs = 0
			}
		}

		if f.ServiceURL != "" {
			maybeAddNL()
			fmt.Fprintf(w, "$ServiceURL %s", f.ServiceURL)
			needsNLs = 1
		}
		if f.ParanoidMode != "" && f.ParanoidMode != deployer.NotParanoid {
			maybeAddNL()
			fmt.Fprintf(w, "$ParanoidMode %s", f.ParanoidMode)
			needsNLs = 1
		}
		if f.ResolvedVersions != "" {
			maybeAddNL()
			fmt.Fprintf(w, "$ResolvedVersions %s", f.ResolvedVersions)
			needsNLs = 1
		}
		if f.OverrideInstallMode != "" {
			maybeAddNL()
			fmt.Fprintf(w, "$OverrideInstallMode %s", f.OverrideInstallMode)
			needsNLs = 1
		}

		if needsNLs != 0 {
			needsNLs++ // new line separator if any of $Directives were used
		}

		if len(f.VerifyPlatforms) > 0 {
			for _, plat := range f.VerifyPlatforms {
				maybeAddNL()

				fmt.Fprintf(w, "$VerifiedPlatform %s", plat.String())
				needsNLs = 1
			}

			needsNLs = 2
		}

		keys := make(sort.StringSlice, 0, len(f.PackagesBySubdir))
		for k := range f.PackagesBySubdir {
			keys = append(keys, k)
		}
		keys.Sort()

		for _, k := range keys {
			maybeAddNL()
			if k != "" {
				fmt.Fprintf(w, "@Subdir %s", k)
				needsNLs = 1
			}

			pkgs := f.PackagesBySubdir[k]
			pkgsSort := make(PackageSlice, len(pkgs))
			maxLength := 0
			for i, pkg := range pkgs {
				pkgsSort[i] = pkg
				if l := len(pkg.PackageTemplate); l > maxLength {
					maxLength = l
				}
			}
			sort.Sort(pkgsSort)

			for _, p := range pkgsSort {
				maybeAddNL()
				fmt.Fprintf(w, "%-*s %s", maxLength+1, p.PackageTemplate, p.UnresolvedVersion)
				needsNLs = 1
			}
			needsNLs++
		}

		// We only ever want to end the file with 1 newline.
		if needsNLs > 0 {
			needsNLs = 1
		}
		maybeAddNL()
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to write resolved ensure file").Tag(cipderr.IO).Err()
	}
	return nil
}
