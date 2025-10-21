// Copyright 2021 The LUCI Authors.
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

package accesslog

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	logpb "go.chromium.org/luci/cipd/api/cipd/v1/logpb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

func TestFields(t *testing.T) {
	t.Parallel()

	obj := &caspb.ObjectRef{
		HashAlgo:  caspb.HashAlgo_SHA256,
		HexDigest: strings.Repeat("a", 64),
	}
	iid := common.ObjectRefToInstanceID(obj)
	pfx := "some/prefix"
	pkg := "some/package"

	tags := []*repopb.Tag{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	tagsStr := []string{"k1:v1", "k2:v2"}

	ref := "some-ref"

	md := []*repopb.InstanceMetadata{
		{Key: "mk1", Value: []byte("zzz")},
		{Key: "mk2", Value: []byte("zzz")},
	}
	mdStr := []string{"mk1", "mk2"}

	cases := []struct {
		req any
		exp *logpb.AccessLogEntry
	}{
		{
			&caspb.GetObjectURLRequest{Object: obj},
			&logpb.AccessLogEntry{Instance: iid},
		},
		{
			&caspb.BeginUploadRequest{Object: obj},
			&logpb.AccessLogEntry{Instance: iid},
		},
		{
			&repopb.PrefixRequest{Prefix: pfx},
			&logpb.AccessLogEntry{Package: pfx},
		},
		{
			&repopb.PrefixMetadata{Prefix: pfx},
			&logpb.AccessLogEntry{Package: pfx},
		},
		{
			&repopb.ListPrefixRequest{Prefix: pfx},
			&logpb.AccessLogEntry{Package: pfx},
		},
		{
			&repopb.PackageRequest{Package: pkg},
			&logpb.AccessLogEntry{Package: pkg},
		},
		{
			&repopb.Instance{Package: pkg, Instance: obj},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid},
		},
		{
			&repopb.ListInstancesRequest{Package: pkg},
			&logpb.AccessLogEntry{Package: pkg},
		},
		{
			&repopb.SearchInstancesRequest{Package: pkg, Tags: tags},
			&logpb.AccessLogEntry{Package: pkg, Tags: tagsStr},
		},
		{
			&repopb.Ref{Package: pkg, Instance: obj, Name: ref},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid, Version: ref},
		},
		{
			&repopb.DeleteRefRequest{Package: pkg, Name: ref},
			&logpb.AccessLogEntry{Package: pkg, Version: ref},
		},
		{
			&repopb.ListRefsRequest{Package: pkg},
			&logpb.AccessLogEntry{Package: pkg},
		},
		{
			&repopb.AttachTagsRequest{Package: pkg, Instance: obj, Tags: tags},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid, Tags: tagsStr},
		},
		{
			&repopb.DetachTagsRequest{Package: pkg, Instance: obj, Tags: tags},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid, Tags: tagsStr},
		},
		{
			&repopb.AttachMetadataRequest{Package: pkg, Instance: obj, Metadata: md},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid, Metadata: mdStr},
		},
		{
			&repopb.DetachMetadataRequest{Package: pkg, Instance: obj, Metadata: md},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid, Metadata: mdStr},
		},
		{
			&repopb.ListMetadataRequest{Package: pkg, Instance: obj, Keys: mdStr},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid, Metadata: mdStr},
		},
		{
			&repopb.ResolveVersionRequest{Package: pkg, Version: ref},
			&logpb.AccessLogEntry{Package: pkg, Version: ref},
		},
		{
			&repopb.GetInstanceURLRequest{Package: pkg, Instance: obj},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid},
		},
		{
			&repopb.DescribeInstanceRequest{
				Package:            pkg,
				Instance:           obj,
				DescribeRefs:       true,
				DescribeTags:       true,
				DescribeProcessors: true,
				DescribeMetadata:   true,
			},
			&logpb.AccessLogEntry{
				Package:  pkg,
				Instance: iid,
				Flags:    []string{"refs", "tags", "processors", "metadata"},
			},
		},
		{
			&repopb.DescribeClientRequest{Package: pkg, Instance: obj},
			&logpb.AccessLogEntry{Package: pkg, Instance: iid},
		},
	}

	for _, c := range cases {
		got := &logpb.AccessLogEntry{}
		extractFieldsFromRequest(got, c.req)
		if !proto.Equal(got, c.exp) {
			t.Errorf("%s: %s != %s", c.req, got, c.exp)
		}
	}
}
