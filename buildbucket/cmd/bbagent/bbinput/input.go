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
	"io"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// Parse returns proto.Unmarshal(zlibDec(base64Dec(encodedData))).
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
	decompressed, err := io.ReadAll(decompressing)
	if err != nil {
		return nil, errors.Annotate(err, "decompressing zlib").Err()
	}

	ret := &bbpb.BBAgentArgs{}
	return ret, errors.WrapIf(proto.Unmarshal(decompressed, ret), "parsing proto")
}

// Encode returns base64(zlib(proto.Marshal(args))).
func Encode(args *bbpb.BBAgentArgs) string {
	pbuf := proto.NewBuffer(nil)
	pbuf.SetDeterministic(true)
	err := pbuf.Marshal(args)
	if err != nil {
		// can only happen if there are unmarshalable extensions in args, which
		// isn't possible.
		panic("impossible: " + err.Error())
	}

	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		// can only happen if zlib.BestCompression is an invalid value.
		panic("impossible: " + err.Error())
	}

	if _, err = zw.Write(pbuf.Bytes()); err != nil {
		// can only happen if buf.Write returns an error
		panic("impossible: " + err.Error())
	}
	if err = zw.Close(); err != nil {
		// can only happen if buf.Close returns an error
		panic("impossible: " + err.Error())
	}

	return base64.RawStdEncoding.EncodeToString(buf.Bytes())
}
