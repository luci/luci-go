// Copyright 2015 The LUCI Authors.
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

//go:generate cproto

// Package versioncli implements a subcommand for obtaining version with the CLI.
//
package versioncli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/cipd/version"
)

// Option can be used add additional features to the version output message.
type Option func(*VersionData)

// WithFeature returns an Option that adds a 'feature flag' section to the
// version output.
//
// `feature` is the name of the feature, and `enabled` indicates if the feature
// is built into this binary or not. When CmdVersion prints the feature data, it
// will indicate enabled features with '+feature_name' and disabled features
// with '-feature_name'.
//
// The feature list will be printed underneath the version and CIPD package
// information.
func WithFeature(feature string, enabled bool) Option {
	return func(v *VersionData) {
		if v.Features == nil {
			v.Features = map[string]bool{}
		}
		v.Features[feature] = enabled
	}
}

func fixSymver(symver string) string {
	if symver == "" {
		panic(fmt.Errorf("empty symver provided: %q", symver))
	}

	toks := strings.Split(symver, ".")
	if len(toks) > 3 {
		panic(fmt.Errorf("more than 2 `.` in symver: %q", symver))
	}

	for i, tok := range toks {
		if _, err := strconv.Atoi(tok); err != nil {
			panic(fmt.Errorf(
				"could not convert token %d of %q to int: %s", i, symver, err))
		}
	}
	switch len(toks) {
	case 2:
		toks = append(toks, "0")
		fallthrough
	case 1:
		toks = append(toks, "0")
	}
	return strings.Join(toks, ".")
}

// CmdVersion returns a "version" subcommand that prints to stdout the given
// version as well as CIPD package name and the package instance ID if the
// executable was installed via CIPD.
//
// If the executable didn't come from CIPD, the package information is silently
// omitted.
//
// 'app' will be printed exactly as given and must not be empty. Should be the
// name of this application, as you would invoke it on the command line.
//
// 'symver' must conform to `MAJOR[.MINOR[.PATCH]]` where MAJOR, MINOR and PATCH
// are numbers. Providing something else will panic. If MINOR or PATCH are
// omitted, they'll be set to 0.
func CmdVersion(app string, symver string, options ...Option) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "version",
		ShortDesc: "prints the executable version",
		LongDesc:  "Prints the executable version and the CIPD package the executable was installed from (if it was installed via CIPD).",
		CommandRun: func() subcommands.CommandRun {
			symver = fixSymver(symver)

			ret := &versionRun{
				data: &VersionData{
					AppName: app,
					Symver:  symver,
				},
			}
			for _, option := range options {
				option(ret.data)
			}
			ret.Flags.BoolVar(&ret.outputJSON, "json", false,
				"Output a JSON version manifest.")
			return ret
		},
	}
}

type versionRun struct {
	subcommands.CommandRunBase

	outputJSON bool

	data *VersionData
}

func (c *versionRun) printHuman() int {
	fmt.Printf("%s v%s", c.data.AppName, c.data.Symver)

	if ci := c.data.GetCipdInfo(); ci != nil {
		fmt.Println()
		fmt.Printf("CIPD package name: %s\n", ci.CipdPackage)
		fmt.Printf("CIPD instance ID:  %s\n", ci.CipdInstanceId)
	}

	if len(c.data.Features) > 0 {
		fmt.Println()
		fmt.Printf("Features:\n")
		featureNames := make([]string, 0, len(c.data.Features))
		for feature := range c.data.Features {
			featureNames = append(featureNames, feature)
		}
		sort.Strings(featureNames)
		for _, feature := range featureNames {
			flag := "-"
			if c.data.Features[feature] {
				flag = "+"
			}
			fmt.Printf("  %s%s\n", flag, feature)
		}
	}
	return 0
}

func (c *versionRun) printJSON() int {
	if err := (&jsonpb.Marshaler{}).Marshal(os.Stdout, c.data); err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %s", err)
		return 1
	}
	return 0
}

func (c *versionRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: positionl arguments not expected\n", a.GetName())
		return 1
	}

	switch ver, err := version.GetStartupVersion(); {
	case err != nil:
		// Note: this is some sort of catastrophic error. If the binary is not
		// installed via CIPD, err == nil && ver.InstanceID == "".
		fmt.Fprintf(os.Stderr, "cannot determine CIPD package version: %s\n", err)
		return 1
	case ver.InstanceID != "":
		c.data.CipdInfo = &VersionData_CIPDVersionInfo{
			CipdPackage:    ver.PackageName,
			CipdInstanceId: ver.InstanceID,
		}
	}

	if c.outputJSON {
		return c.printJSON()
	}
	return c.printHuman()
}
