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

package bqexporter

import (
	"fmt"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/runtime/protoiface"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var artifactRowSchema bigquery.Schema

const (
	artifactRowMessage = "luci.resultdb.bq.TextArtifactRow"

	// Row size limit is 5MB according to
	// https://cloud.google.com/bigquery/quotas#streaming_inserts
	// Split artifact content into 4MB shards if it's too large.
	contentShardSize = 4e6
)

func init() {
	var err error
	if artifactRowSchema, err = generateArtifactRowSchema(); err != nil {
		panic(err)
	}
}

func generateArtifactRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TextArtifactRow{})
	fdinv, _ := descriptor.MessageDescriptorProto(&bqpb.InvocationRecord{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdinv, fdsp}}
	return generateSchema(fdset, artifactRowMessage)
}

// textArtifactRowInput is information required to generate a text artifact BigQuery row.
type textArtifactRowInput struct {
	exported *pb.Invocation
	parent   *pb.Invocation
	a        *pb.Artifact
	shardID  int32
	content  string
}

func (i *textArtifactRowInput) row() protoiface.MessageV1 {
	_, testID, resultID, artifactID := artifacts.MustParseName(i.a.Name)
	expRec := invocationProtoToRecord(i.exported)
	parRect := invocationProtoToRecord(i.parent)

	return &bqpb.TextArtifactRow{
		Exported:   expRec,
		Parent:     parRect,
		TestId:     testID,
		ResultId:   resultID,
		ArtifactId: artifactID,
		ShardId:    i.shardID,
		Content:    i.content,
	}
}

func (i *textArtifactRowInput) id() []byte {
	return []byte(fmt.Sprintf("%s/%d", i.a.Name, i.shardID))
}
