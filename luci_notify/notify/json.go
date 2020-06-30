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

package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
)

var marshaller = jsonpb.Marshaler{}

type envelope struct {
	Type string           `json:"type"`
	Body *json.RawMessage `json:"body"`
}

func serializePayload(task proto.Message) ([]byte, error) {
	var buf bytes.Buffer
	if err := marshaller.Marshal(&buf, task); err != nil {
		return nil, err
	}
	raw := json.RawMessage(buf.Bytes())
	return json.Marshal(envelope{
		Type: proto.MessageName(task),
		Body: &raw,
	})
}

func deserializePayload(blob []byte) (proto.Message, error) {
	env := envelope{}
	if err := json.Unmarshal(blob, &env); err != nil {
		return nil, err
	}

	tp := proto.MessageType(env.Type) // this is **ConcreteStruct{}
	if tp == nil {
		return nil, fmt.Errorf("unregistered proto message name %q", env.Type)
	}
	if env.Body == nil {
		return nil, fmt.Errorf("no task body given")
	}

	task := reflect.New(tp.Elem()).Interface().(proto.Message)
	if err := jsonpb.Unmarshal(bytes.NewReader(*env.Body), task); err != nil {
		return nil, err
	}

	return task, nil
}
