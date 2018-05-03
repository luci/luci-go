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

package main

import (
	"bytes"
	"net/http"
	"text/template"

	machine "go.chromium.org/luci/common/api/machine_provider/machine/v1"

	"golang.org/x/net/context"
)

// MachineProvider encapsulates a client for interacting with Machine Provider.
type MachineProvider struct {
	client *machine.Service
}

// getClient returns a new instance of the MachineProvider client.
func getClient(ctx context.Context, client *http.Client, server string) (*MachineProvider, error) {
	mp, err := machine.New(client)
	if err != nil {
		return nil, err
	}
	mp.BasePath = server + "/api/machine/v1/"
	return &MachineProvider{client: mp}, nil
}

// ack acknowledges receipt and execution of a Machine Provider instruction.
func (mp *MachineProvider) ack(ctx context.Context, hostname, backend string) error {
	return mp.client.Ack(&machine.ComponentsMachineProviderRpcMessagesAckRequest{
		Backend:  backend,
		Hostname: hostname,
	}).Context(ctx).Do()
}

// poll polls Machine Provider for instructions.
//
// Returns an Instruction.
func (mp *MachineProvider) poll(ctx context.Context, hostname, backend string) (*machine.ComponentsMachineProviderRpcMessagesPollResponse, error) {
	return mp.client.Poll(&machine.ComponentsMachineProviderRpcMessagesPollRequest{
		Backend:  backend,
		Hostname: hostname,
	}).Context(ctx).Do()
}

// substitute reads a template string and performs substitutions.
//
// Returns a string.
func substitute(ctx context.Context, templateString string, substitutions interface{}) (string, error) {
	template, err := template.New(templateString).Parse(templateString)
	if err != nil {
		return "", err
	}
	buffer := bytes.Buffer{}
	if err = template.Execute(&buffer, substitutions); err != nil {
		return "", nil
	}
	return buffer.String(), nil
}

// substitute asset reads an asset string and performs substitutions.
//
// Returns a string.
func substituteAsset(ctx context.Context, asset string, substitutions interface{}) (string, error) {
	return substitute(ctx, string(GetAsset(asset)), substitutions)
}
