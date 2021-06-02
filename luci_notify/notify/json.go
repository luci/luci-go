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
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type envelope struct {
	Type protoreflect.FullName `json:"type"`
	Body *json.RawMessage      `json:"body"`
}

func serializePayload(task proto.Message) ([]byte, error) {
	buf, err := protojson.Marshal(task)
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(buf)
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

	tp, err := protoregistry.GlobalTypes.FindMessageByName(env.Type) // this is **ConcreteStruct{}
	if err != nil {
		return nil, fmt.Errorf("unregistered proto message name %q: %w", env.Type, err)
	}
	if env.Body == nil {
		return nil, fmt.Errorf("no task body given")
	}

	task := tp.New().Interface()
	if err := protojson.Unmarshal(*env.Body, task); err != nil {
		return nil, err
	}

	return task, nil
}
