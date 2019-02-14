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

// Package versioncli implements a subcommand for obtaining version with the CLI.
//
package versioncli

import (
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/cipd/version"
)

// CmdVersion returns a "version" subcommand that prints to stdout the given
// version as well as CIPD package name and the package instance ID if the
// executable was installed via CIPD.
//
// If the executable didn't come from CIPD, the package information is silently
// omitted.
//
// 'version' will be printed exact as given. The recommended format is
// "<appname> vMAJOR.MINOR.PATCH".
func CmdVersion(version string) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "version",
		ShortDesc: "prints the executable version",
		LongDesc:  "Prints the executable version and the CIPD package the executable was installed from (if it was installed via CIPD).",
		CommandRun: func() subcommands.CommandRun {
			return &versionRun{version: version}
		},
	}
}

type versionRun struct {
	subcommands.CommandRunBase
	version string
}

func (c *versionRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: position arguments not expected\n", a.GetName())
		return 1
	}
	fmt.Println(c.version)

	switch ver, err := version.GetStartupVersion(); {
	case err != nil:
		// Note: this is some sort of catastrophic error. If the binary is not
		// installed via CIPD, err == nil && ver.InstanceID == "".
		fmt.Fprintf(os.Stderr, "cannot determine CIPD package version: %s\n", err)
		return 1
	case ver.InstanceID != "":
		fmt.Println()
		fmt.Printf("CIPD package name: %s\n", ver.PackageName)
		fmt.Printf("CIPD instance ID:  %s\n", ver.InstanceID)
	}

	return 0
}
