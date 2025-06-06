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

package ledcli

import (
	"context"
	"fmt"
	"net/http"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func editPayloadCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-payload [options]",
		ShortDesc: "edit the job payload with provided RBE-CAS reference or CIPD info",
		LongDesc: `edit the job payload with provided RBE-CAS reference or CIPD info.
By default, the provided payload will be used as the led job's user payload.
If the -property-only flag is passed or the builder has the "led_builder_is_bootstrapped"
property set to true, the "led_cas_recipe_bundle" property will be set with the CAS digest so
that the build's bootstrapper executable can launch the bundled recipes.
`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditPayload{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdEditPayload struct {
	cmdBase

	propertyOnly bool

	casRef    string
	casDigest *swarmingpb.Digest

	cipdPkg string
	cipdVer string
}

func (c *cmdEditPayload) initFlags(opts cmdBaseOptions) {
	c.Flags.BoolVar(&c.propertyOnly, "property-only", false,
		fmt.Sprintf("Pass the CAS reference via the %q property and "+
			"preserve the executable of the input job rather than overwriting it. This "+
			"is useful for when `exe` is actually a bootstrap program that you don't "+
			"want to change. The same behavior can be enabled for a build without this "+
			"flag by setting the \"led_builder_is_bootstrapped\" property to true.",
			ledcmd.CASRecipeBundleProperty))

	c.Flags.StringVar(&c.casRef, "cas-ref", "", text.Doc(`
		The RBE-CAS reference of the payload, in the format of "hash/size",
		e.g. "dead...beef/1234".
	`))

	c.Flags.StringVar(&c.cipdPkg, "cipd-pkg", "",
		"Name of the CIPD package to be used as user payload.")
	c.Flags.StringVar(&c.cipdVer, "cipd-ver", "",
		"Version of the CIPD package to be used as user payload.")

	c.cmdBase.initFlags(opts)
}

func (c *cmdEditPayload) jobInput() bool                  { return true }
func (c *cmdEditPayload) positionalRange() (min, max int) { return 0, 0 }

func (c *cmdEditPayload) validateFlags(ctx context.Context, _ []string, _ subcommands.Env) (err error) {
	if c.casRef != "" {
		if c.casDigest, err = job.ToCasDigest(c.casRef); err != nil {
			return
		}
	}
	if c.propertyOnly && (c.cipdPkg != "" || c.cipdVer != "") {
		return errors.New("cannot use payload from CIPD in -property-only mode")
	}
	return
}

func (c *cmdEditPayload) execute(ctx context.Context, _ *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	return inJob, ledcmd.EditPayload(ctx, inJob, &ledcmd.EditPayloadOpts{
		PropertyOnly: c.propertyOnly,
		CasDigest:    c.casDigest,
		CIPDPkg:      c.cipdPkg,
		CIPDVer:      c.cipdVer,
	})
}

func (c *cmdEditPayload) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
