// Copyright 2018 The LUCI Authors.
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

package swarmingimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"google.golang.org/api/googleapi"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/system/signals"

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
	"google.golang.org/protobuf/encoding/protojson"
)

// CmdBots returns an object for the `bots` subcommand.
func CmdBots(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "bots <options>",
		ShortDesc: "lists bots",
		LongDesc:  "List bots matching the given options.",
		CommandRun: func() subcommands.CommandRun {
			r := &botsRun{}
			r.Init(authFlags)
			return r
		},
	}
}

type botsRun struct {
	commonFlags
	outfile    string
	dimensions stringmapflag.Value
	fields     []googleapi.Field
	count      bool
}

func (b *botsRun) Init(authFlags AuthFlags) {
	b.commonFlags.Init(authFlags)
	b.Flags.StringVar(&b.outfile, "json", "", "Path to output JSON results. Implies quiet.")
	b.Flags.Var(&b.dimensions, "dimension", "Dimension to select the right kind of bot. In the form of `key=value`")
	b.Flags.Var(flag.FieldSlice(&b.fields), "field", `Fields to include in a partial response. May be repeated.
See https://pkg.go.dev/google.golang.org/api/googleapi#Field for more explanation`)
	b.Flags.BoolVar(&b.count, "count", false, "Report the count of bots instead of listing them.")
}

func (b *botsRun) Parse() error {
	if err := b.commonFlags.Parse(); err != nil {
		return err
	}
	if b.defaultFlags.Quiet && b.outfile == "" {
		return errors.Reason("specify -json when using -quiet").Err()
	}
	if b.outfile != "" {
		b.defaultFlags.Quiet = true
	}
	if b.count && len(b.fields) > 0 {
		return errors.Reason("-field cannot be used with -count").Err()
	}
	return nil
}

type botInfo struct {
	options *protojson.MarshalOptions
	bot     *swarmingv2.BotInfo
}

func (b *botInfo) MarshalJSON() ([]byte, error) {
	return b.options.Marshal(b.bot)
}

func showBots(bots []*swarmingv2.BotInfo) ([]byte, error) {
	botInfos := make([]botInfo, len(bots))
	options := DefaultProtoMarshalOpts()
	for i, bot := range bots {
		botInfos[i] = botInfo{
			bot:     bot,
			options: &options,
		}
	}
	return json.MarshalIndent(botInfos, "", options.Indent)
}

func (b *botsRun) bots(ctx context.Context, service swarmingService, out io.Writer) error {
	dims := make([]*swarmingv2.StringPair, 0, len(b.dimensions))
	for k, v := range b.dimensions {
		dims = append(dims, &swarmingv2.StringPair{
			Key:   k,
			Value: v,
		})
	}

	var output []byte
	if b.count {
		count, err := service.CountBots(ctx, dims)
		if err != nil {
			return err
		}
		output, err = DefaultProtoMarshalOpts().Marshal(count)
		if err != nil {
			return err
		}

	} else {
		bots, err := service.ListBots(ctx, dims)
		if err != nil {
			return err
		}
		output, err = showBots(bots)
		if err != nil {
			return err
		}
	}
	_, err := out.Write(append(output, '\n'))
	if err != nil {
		return err
	}
	return nil
}

func (b *botsRun) main(_ subcommands.Application) (err error) {
	ctx, cancel := context.WithCancel(b.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	service, err := b.createSwarmingClient(ctx)
	if err != nil {
		return err
	}
	writers := make([]io.Writer, 0, 2)
	if !b.defaultFlags.Quiet {
		writers = append(writers, os.Stdout)
	}
	if b.outfile != "" {
		f, err := os.OpenFile(b.outfile, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				if err == nil {
					err = closeErr
				} else {
					err = errors.Append(closeErr, err)
				}
			}
		}()
		writers = append(writers, f)
	}
	out := io.MultiWriter(writers...)
	return b.bots(ctx, service, out)
}

func (b *botsRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if len(args) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: unknown args: %s\n", a.GetName(), args)
		return 1
	}
	if err := b.Parse(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := b.main(a); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}
