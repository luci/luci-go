// Copyright 2019 The LUCI Authors.
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
	"github.com/golang/protobuf/proto"

	luciproto "go.chromium.org/luci/common/proto"
)

func textpb(pb proto.Message, textML string) proto.Message {
	if err := luciproto.UnmarshalTextML(textML, pb); err != nil {
		panic(err)
	}
	return pb
}
