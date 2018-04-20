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

package skylarkproto

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/google/skylark"
)

// messageMethods contains method exposed by all Message objects.
//
// They each take Message as a first argument.
var messageMethods = map[string]*skylark.Builtin{
	"to_text": skylark.NewBuiltin("to_text", func(_ *skylark.Thread, bt *skylark.Builtin, args skylark.Tuple, kwargs []skylark.Tuple) (skylark.Value, error) {
		if len(args) != 0 || len(kwargs) != 0 {
			return nil, fmt.Errorf("\"to_text\" expects no arguments, got %d", len(args)+len(kwargs))
		}
		self := bt.Receiver().(*Message)
		msg, err := self.ToProto()
		if err != nil {
			return nil, err
		}
		return skylark.String(proto.MarshalTextString(msg)), nil
	}),
}
