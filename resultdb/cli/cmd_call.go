// Copyright 2020 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
)

const callUsage = `call [flags] SERVICE METHOD`

func cmdCall(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: callUsage,
		ShortDesc: "call a ResultDB RPC",
		LongDesc: text.Doc(`
			Call a ResultDB RPC.

			SERVICE must be the full name of a service, e.g. "luci.resultdb.rpc.v1.ResultDB"".
			METHOD is the name of the method, e.g. "GetInvocation"

			The request message is read from stdin, in JSON format.
			The rtnponse is printed to stdout, also in JSON format.
		`),
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			r := &callRun{}
			r.RegisterGlobalFlags(p)
			return r
		},
	}
}

type callRun struct {
	baseCommandRun
	service string
	method  string
}

func (r *callRun) parseArgs(args []string) error {
	if len(args) < 2 {
		return errors.Reason("usage: %s", callUsage).Err()
	}

	r.service = args[0]
	r.method = args[1]

	return nil
}

func (r *callRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	var rtn proto.Message
	var err error
	switch r.service {
	case "luci.resultdb.rpc.v1.Deriver":
		rtn, err = r.call(ctx, r.deriver, false)
	case "luci.resultdb.rpc.v1.Recorder":
		rtn, err = r.call(ctx, r.recorder, true)
	case "luci.resultdb.rpc.v1.ResultDB":
		rtn, err = r.call(ctx, r.resultdb, false)
	default:
		return r.done(fmt.Errorf("unknown service"))
	}
	if err != nil {
		return r.done(err)
	}
	r.print(rtn)
	return 0
}

func (r *callRun) call(ctx context.Context, client interface{}, requireUpdateToken bool) (proto.Message, error) {
	m := reflect.ValueOf(client).MethodByName(r.method)
	if !m.IsValid() {
		return nil, fmt.Errorf("method %s not found in service %s", r.method, r.service)
	}

	// Expect at least context and request message.
	if m.Type().NumIn() < 2 {
		return nil, fmt.Errorf("method %s doesn not have enough parameters", r.method)
	}

	// Prepare input arguments.
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(os.Stdin); err != nil {
		return nil, err
	}
	in := reflect.New(m.Type().In(1).Elem()).Interface().(proto.Message)
	unmarshaller := jsonpb.Unmarshaler{AllowUnknownFields: false}
	unmarshaller.Unmarshal(bytes.NewReader(buf.Bytes()), in)

	if requireUpdateToken {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			"update-token", r.resultdbCtx.CurrentInvocation.UpdateToken))
	}

	callValues := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(in),
	}

	// Call returns results as reflect.Values.
	result := m.Call(callValues)
	// Expect response message and error.
	if len(result) < 2 {
		return nil, fmt.Errorf("method %s does not have enough return values", r.method)
	}

	if !result[1].IsZero() { // err != nil
		return nil, result[1].Interface().(error)
	}
	return result[0].Interface().(proto.Message), nil
}

func (r *callRun) print(msg proto.Message) {
	enc := json.NewEncoder(os.Stdout)
	jsonMsg := json.RawMessage(msgToJSON(msg))
	enc.Encode(jsonMsg) // prints \n in the end
}
