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

// Contains utility functions.
package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"text/template"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

// Represents a response to a request to poll for instructions.
//
// https://chromium.googlesource.com/infra/luci/luci-py/+/master/appengine/components/components/machine_provider/rpc_messages.py
type Instruction struct {
	Instruction struct {
		SwarmingServer string	`json:"swarming_server"`
	}		`json:"instruction"`
	State string	`json:"state"`
}

// Acknowledges receipt and execution of a Machine Provider instruction.
func ack(ctx context.Context, client *http.Client, url string, body []byte) error {
	response, err := client.Post(url, "application/json", bytes.NewReader(body))
	if (err != nil) {
		return err
	}
	defer response.Body.Close()
	if (response.Status != "204 No Content") {
		// TODO(smut): Differentiate between transient and non-transient.
		return errors.New("Unexpected HTTP status: " + response.Status + ".")
	}
	return nil
}

// Polls Machine Provider for instructions.
//
// Returns an Instruction.
func poll(ctx context.Context, client *http.Client, url string, body []byte) (*Instruction, error) {
	response, err := client.Post(url, "application/json", bytes.NewReader(body))
	if (err != nil) {
		return nil, err
	}
	defer response.Body.Close()
	if (response.Status != "200 OK") {
		// TODO(smut): Differentiate between transient and non-transient.
		return nil, errors.New("Unexpected HTTP status: " + response.Status + ".")
	}
	bytes, err := ioutil.ReadAll(response.Body)
	if (err != nil) {
		return nil, err
	}
	var instruction = Instruction{}
	err = json.Unmarshal(bytes, &instruction)
	if (err != nil) {
		return nil, err
	}
	return &instruction, nil
}

// Reads a template string and performs substitutions.
//
// Returns a string.
func substitute(ctx context.Context, templateString string, substitutions interface{}) (string, error) {
	template, err := template.New(templateString).Parse(templateString)
	if (err != nil) {
		return "", err
	}
	buffer := bytes.Buffer{}
	err = template.Execute(&buffer, substitutions)
	if (err != nil) {
		return "", nil
	}
	return buffer.String(), nil
}

// Reads a template file and performs substitutions.
//
// Returns a string.
func substituteFile(ctx context.Context, path string, substitutions interface{}) (string, error) {
	bytes, err := ioutil.ReadFile(path)
	if (err != nil) {
		return "", err
	}
	return substitute(ctx, string(bytes), substitutions)
}
