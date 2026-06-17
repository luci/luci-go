// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestComputeDigest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  proto.Message
		want Digest
	}{
		{
			"empty",
			&emptypb.Empty{},
			"zC1HiB0gq_T1muuCh5VIAoC4FjWvxp00E9waqU1YMhkrAQ",
		},
		{
			"float_val",
			structpb.NewNumberValue(123.456),
			"aexUjcBYp_UhSBsbm6TwadrRm0ZAYUrR5mRAKiJ2XtQ2AQ",
		},
		{
			"long_string",
			structpb.NewStringValue(strings.Repeat("this is a very long string", 40000)),
			"tvpg39g5kBqzdMKxPOWxvE82_CR13ZmPUmuaq186WyCzvT8B",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			apb, err := anypb.New(tc.msg)
			assertNoErr(t, err)

			dgst := ComputeDigest(apb)
			assertEqual(t, tc.want, dgst)

			dgstPb, err := dgst.ToProto()
			assertNoErr(t, err)

			wantSize := proto.Size(apb)
			enc, err := proto.Marshal(apb)
			assertNoErr(t, err)
			assertEqual(t, wantSize, len(enc))

			detEnc := DeterministicallySerializeAny(apb)
			assertLen(t, detEnc, wantSize)

			dec := &anypb.Any{}
			assertNoErr(t, proto.Unmarshal(detEnc, dec))

			assertTrue(t, proto.Equal(dec, apb))

			sha := sha256.Sum256(detEnc)
			assertMatch(t, sha[:], dgstPb.GetHash())

			assertEqual(t, uint64(wantSize), dgstPb.GetSizeBytes())
		})
	}
}

func TestDigestToProtoErrors(t *testing.T) {
	t.Parallel()

	b64 := func(hsh, size, algo []byte) Digest {
		toEnc := make([]byte, 0, len(hsh)+len(size)+len(algo))
		toEnc = append(toEnc, hsh...)
		toEnc = append(toEnc, size...)
		toEnc = append(toEnc, algo...)
		return Digest(base64.RawURLEncoding.EncodeToString(toEnc))
	}

	cases := []struct {
		name    string
		digest  Digest
		wantErr string
	}{
		{
			name:    "bad_base64",
			digest:  "heloworld",
			wantErr: "illegal base64",
		},
		{
			name:    "missing_algo",
			digest:  "",
			wantErr: "missing algorithm",
		},
		{
			name:    "bad_algo",
			digest:  b64(make([]byte, 32), []byte{1}, []byte{32}),
			wantErr: "bad algorithm",
		},
		{
			name:    "small_hash",
			digest:  b64(make([]byte, 20), []byte{1}, []byte{byte(orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256)}),
			wantErr: "insufficient bytes for hash",
		},
		{
			name:    "extra_size",
			digest:  b64(make([]byte, 32), []byte{1, 1, 1}, []byte{byte(orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256)}),
			wantErr: "extra bytes while decoding size",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.digest.ToProto()
			assertErrLike(t, err, tc.wantErr)
		})
	}
}
