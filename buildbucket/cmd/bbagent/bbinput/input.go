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

package bbinput

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

// Parse parses the base64(zlib(proto.Marshal(BBAgentArgs))) `encodedData`.
func Parse(encodedData string) (*bbpb.BBAgentArgs, error) {
	if encodedData == "" {
		return nil, errors.New("inputs required")
	}

	compressed, err := base64.RawStdEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, errors.Annotate(err, "decoding base64").Err()
	}

	decompressing, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, errors.Annotate(err, "opening zlib reader").Err()
	}
	decompressed, err := ioutil.ReadAll(decompressing)
	if err != nil {
		return nil, errors.Annotate(err, "decompressing zlib").Err()
	}

	ret := &bbpb.BBAgentArgs{}
	return ret, errors.Annotate(proto.Unmarshal(decompressed, ret), "parsing proto").Err()
}
