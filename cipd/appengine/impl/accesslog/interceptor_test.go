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

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

func TestFields(t *testing.T) {
	t.Parallel()

	obj := &cipdpb.ObjectRef{
		HashAlgo:  cipdpb.HashAlgo_SHA256,
		HexDigest: strings.Repeat("a", 64),
	}
	iid := common.ObjectRefToInstanceID(obj)
	pfx := "some/prefix"
	pkg := "some/package"

	tags := []*cipdpb.Tag{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	tagsStr := []string{"k1:v1", "k2:v2"}

	ref := "some-ref"

	md := []*cipdpb.InstanceMetadata{
		{Key: "mk1", Value: []byte("zzz")},
		{Key: "mk2", Value: []byte("zzz")},
	}
	mdStr := []string{"mk1", "mk2"}

	cases := []struct {
		req any
		exp *cipdpb.AccessLogEntry
	}{
		{
			&cipdpb.GetObjectURLRequest{Object: obj},
			&cipdpb.AccessLogEntry{Instance: iid},
		},
		{
			&cipdpb.BeginUploadRequest{Object: obj},
			&cipdpb.AccessLogEntry{Instance: iid},
		},
		{
			&cipdpb.PrefixRequest{Prefix: pfx},
			&cipdpb.AccessLogEntry{Package: pfx},
		},
		{
			&cipdpb.PrefixMetadata{Prefix: pfx},
			&cipdpb.AccessLogEntry{Package: pfx},
		},
		{
			&cipdpb.ListPrefixRequest{Prefix: pfx},
			&cipdpb.AccessLogEntry{Package: pfx},
		},
		{
			&cipdpb.PackageRequest{Package: pkg},
			&cipdpb.AccessLogEntry{Package: pkg},
		},
		{
			&cipdpb.Instance{Package: pkg, Instance: obj},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid},
		},
		{
			&cipdpb.ListInstancesRequest{Package: pkg},
			&cipdpb.AccessLogEntry{Package: pkg},
		},
		{
			&cipdpb.SearchInstancesRequest{Package: pkg, Tags: tags},
			&cipdpb.AccessLogEntry{Package: pkg, Tags: tagsStr},
		},
		{
			&cipdpb.Ref{Package: pkg, Instance: obj, Name: ref},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid, Version: ref},
		},
		{
			&cipdpb.DeleteRefRequest{Package: pkg, Name: ref},
			&cipdpb.AccessLogEntry{Package: pkg, Version: ref},
		},
		{
			&cipdpb.ListRefsRequest{Package: pkg},
			&cipdpb.AccessLogEntry{Package: pkg},
		},
		{
			&cipdpb.AttachTagsRequest{Package: pkg, Instance: obj, Tags: tags},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid, Tags: tagsStr},
		},
		{
			&cipdpb.DetachTagsRequest{Package: pkg, Instance: obj, Tags: tags},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid, Tags: tagsStr},
		},
		{
			&cipdpb.AttachMetadataRequest{Package: pkg, Instance: obj, Metadata: md},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid, Metadata: mdStr},
		},
		{
			&cipdpb.DetachMetadataRequest{Package: pkg, Instance: obj, Metadata: md},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid, Metadata: mdStr},
		},
		{
			&cipdpb.ListMetadataRequest{Package: pkg, Instance: obj, Keys: mdStr},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid, Metadata: mdStr},
		},
		{
			&cipdpb.ResolveVersionRequest{Package: pkg, Version: ref},
			&cipdpb.AccessLogEntry{Package: pkg, Version: ref},
		},
		{
			&cipdpb.GetInstanceURLRequest{Package: pkg, Instance: obj},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid},
		},
		{
			&cipdpb.DescribeInstanceRequest{
				Package:            pkg,
				Instance:           obj,
				DescribeRefs:       true,
				DescribeTags:       true,
				DescribeProcessors: true,
				DescribeMetadata:   true,
			},
			&cipdpb.AccessLogEntry{
				Package:  pkg,
				Instance: iid,
				Flags:    []string{"refs", "tags", "processors", "metadata"},
			},
		},
		{
			&cipdpb.DescribeClientRequest{Package: pkg, Instance: obj},
			&cipdpb.AccessLogEntry{Package: pkg, Instance: iid},
		},
	}

	for _, c := range cases {
		got := &cipdpb.AccessLogEntry{}
		extractFieldsFromRequest(got, c.req)
		if !proto.Equal(got, c.exp) {
			t.Errorf("%s: %s != %s", c.req, got, c.exp)
		}
	}
}
