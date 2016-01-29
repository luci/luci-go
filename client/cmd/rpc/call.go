// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
	"google.golang.org/grpc/metadata"

	"github.com/luci/luci-go/common/prpc"
)

// TODO(nodir): support specifying message fields with options, e.g.
//   rpc call :8080 helloworld.Greeter -name Lucy
// It will be the default request format.

var cmdCall = &subcommands.Command{
	UsageLine: `call [flags] <server> <service>.<method>

  server: host ("example.com") or port for localhost (":8080").
  service: full name of a service, e.g. "pkg.service"
  method: name of the method.`,
	ShortDesc: "calls a service method.",
	LongDesc:  "Calls a service method.",
	CommandRun: func() subcommands.CommandRun {
		c := &callRun{
			format: formatFlag(prpc.FormatJSONPB),
		}
		c.registerBaseFlags()
		c.Flags.Var(&c.format, "format", fmt.Sprintf(
			"Request format, read from stdin. Valid values: %s", formatFlagMap.Choices()))
		return c
	},
}

// callRun implements "call" subcommand.
type callRun struct {
	cmdRun
	format  formatFlag
	message string
}

func (r *callRun) Run(a subcommands.Application, args []string) int {
	if len(args) != 2 {
		return r.argErr("")
	}
	host, target := args[0], args[1]

	req := request{
		format: r.format.Format(),
	}

	var err error
	req.service, req.method, err = splitServiceAndMethod(target)
	if err != nil {
		return r.argErr("%s", err)
	}

	var msg bytes.Buffer
	if _, err := msg.ReadFrom(os.Stdin); err != nil {
		return r.done(fmt.Errorf("could not read message from stdin: %s", err))
	}
	req.message = msg.Bytes()

	client, err := r.authenticatedClient(host)
	if err != nil {
		return 2
	}

	return r.run(func(c context.Context) error {
		return call(c, client, &req, os.Stdout)
	})
}

func splitServiceAndMethod(fullName string) (service string, method string, err error) {
	lastDot := strings.LastIndex(fullName, ".")
	if lastDot < 0 {
		return "", "", fmt.Errorf("invalid full method name %q. It must contain a '.'", fullName)
	}
	service = fullName[:lastDot]
	method = fullName[lastDot+1:]
	return
}

// request is an RPC request.
type request struct {
	service string
	method  string
	message []byte
	format  prpc.Format
}

// call makes an RPC and writes response to out.
func call(c context.Context, client *prpc.Client, req *request, out io.Writer) error {
	// Send the request.
	var hmd, tmd metadata.MD
	res, err := client.CallRaw(c, req.service, req.method, req.message, req.format, prpc.Header(&hmd), prpc.Trailer(&tmd))
	if err != nil {
		return err
	}

	// Read response.
	if _, err := out.Write(res); err != nil {
		return fmt.Errorf("failed to write response: %s", err)
	}
	return err
}
