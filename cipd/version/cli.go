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

package version

import (
	"fmt"
	"os"

	"github.com/maruel/subcommands"
)

// SubcommandVersion implement subcommand that prints version of CIPD package
// that contains the executable.
var SubcommandVersion = &subcommands.Command{
	UsageLine:  "version",
	ShortDesc:  "prints version of CIPD package this exe was installed from",
	LongDesc:   "Prints version of CIPD package this exe was installed from.",
	CommandRun: func() subcommands.CommandRun { return &versionRun{} },
}

type versionRun struct {
	subcommands.CommandRunBase
}

func (c *versionRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	ver, err := GetStartupVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if ver.InstanceID == "" {
		fmt.Fprintf(os.Stderr, "Not installed via CIPD package\n")
		return 1
	}
	fmt.Printf("Package name: %s\n", ver.PackageName)
	fmt.Printf("Instance ID:  %s\n", ver.InstanceID)
	return 0
}
