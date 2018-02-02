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

package wheel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Name is a parsed Python wheel name, defined here:
// https://www.python.org/dev/peps/pep-0427/#file-name-convention
//
// {distribution}-{version}(-{build tag})?-{python tag}-{abi tag}-\
// {platform tag}.whl .
type Name struct {
	Distribution string
	Version      string
	BuildTag     string
	PythonTag    string
	ABITag       string
	PlatformTag  string
}

func (wn *Name) String() string {
	parts := make([]string, 0, 6)
	parts = append(parts, []string{
		wn.Distribution,
		wn.Version,
	}...)
	if wn.BuildTag != "" {
		parts = append(parts, wn.BuildTag)
	}
	parts = append(parts, []string{
		wn.PythonTag,
		wn.ABITag,
		wn.PlatformTag,
	}...)
	return strings.Join(parts, "-") + ".whl"
}

// ParseName parses a wheel Name from its filename.
func ParseName(v string) (wn Name, err error) {
	base := strings.TrimSuffix(v, ".whl")
	if len(base) == len(v) {
		err = errors.New("missing .whl suffix")
		return
	}

	skip := 0
	switch parts := strings.Split(base, "-"); len(parts) {
	case 6:
		// Extra part: build tag.
		wn.BuildTag = parts[2]
		skip = 1
		fallthrough

	case 5:
		wn.Distribution = parts[0]
		wn.Version = parts[1]
		wn.PythonTag = parts[2+skip]
		wn.ABITag = parts[3+skip]
		wn.PlatformTag = parts[4+skip]

	default:
		err = errors.Reason("unknown number of segments (%d)", len(parts)).Err()
		return
	}
	return
}

// ScanDir identifies all wheel files in the immediate directory dir and
// returns their parsed wheel names.
func ScanDir(dir string) ([]Name, error) {
	globPattern := filepath.Join(dir, "*.whl")
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return nil, errors.Annotate(err, "failed to list wheel directory: %s", dir).
			InternalReason("pattern(%s)", globPattern).Err()
	}

	names := make([]Name, 0, len(matches))
	for _, match := range matches {
		switch st, err := os.Stat(match); {
		case err != nil:
			return nil, errors.Annotate(err, "failed to stat wheel: %s", match).Err()

		case st.IsDir():
			// Ignore directories.
			continue

		default:
			// A ".whl" file.
			name := filepath.Base(match)
			wheelName, err := ParseName(name)
			if err != nil {
				return nil, errors.Annotate(err, "failed to parse wheel from: %s", name).
					InternalReason("dir(%s)", dir).Err()
			}
			names = append(names, wheelName)
		}
	}
	return names, nil
}

// WriteRequirementsFile writes a valid "requirements.txt"-style pip
// requirements file containing the supplied wheels.
//
// The generated requirements will request the exact wheel senver version (using
// "==").
func WriteRequirementsFile(path string, wheels []Name) (err error) {
	fd, err := os.Create(path)
	if err != nil {
		return errors.Annotate(err, "failed to create requirements file").Err()
	}
	defer func() {
		closeErr := fd.Close()
		if closeErr != nil && err == nil {
			err = errors.Annotate(closeErr, "failed to Close").Err()
		}
	}()

	// Emit a series of "Distribution==Version" strings.
	seen := make(map[Name]struct{}, len(wheels))
	for _, wheel := range wheels {
		// Only mention a given Distribution/Version once.
		archetype := Name{
			Distribution: wheel.Distribution,
			Version:      wheel.Version,
		}
		if _, ok := seen[archetype]; ok {
			// Already seen a package for this archetype, skip it.
			continue
		}
		seen[archetype] = struct{}{}

		if _, err := fmt.Fprintf(fd, "%s==%s\n", archetype.Distribution, archetype.Version); err != nil {
			return errors.Annotate(err, "failed to write to requirements file").Err()
		}
	}

	return nil
}
