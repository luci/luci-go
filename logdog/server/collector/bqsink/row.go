// Copyright 2017 The LUCI Authors.
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

package bqsink

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/storage"
)

// rowToExport defines the structure of bigquery row we export.
type rowToExport struct {
	Project     string    `bigquery:"project"`
	Path        string    `bigquery:"path"`
	Prefix      string    `bigquery:"prefix"`
	Name        string    `bigquery:"name"`
	Tags        []string  `bigquery:"tags"`
	Timestamp   time.Time `bigquery:"timestamp"`
	StreamIndex int64     `bigquery:"stream_index"`
	PrefixIndex int64     `bigquery:"prefix_index"`
	Lines       []string  `bigquery:"lines"`
}

func (r *rowToExport) insertID() string {
	return fmt.Sprintf("%s:%s:%d", r.Project, r.Path, r.StreamIndex)
}

var rowToExportSchema bigquery.Schema

func init() {
	var err error
	rowToExportSchema, err = bigquery.InferSchema(rowToExport{})
	if err != nil {
		panic(err)
	}
}

func toStuctSavers(rows []rowToExport) []*bigquery.StructSaver {
	out := make([]*bigquery.StructSaver, len(rows))
	for i := range rows {
		out[i] = &bigquery.StructSaver{
			Schema:   rowToExportSchema,
			InsertID: rows[i].insertID(),
			Struct:   &rows[i],
		}
	}
	return out
}

func logsToRows(ctx context.Context, desc *logpb.LogStreamDescriptor, r *storage.PutRequest) []rowToExport {
	out := make([]rowToExport, 0, len(r.Values))
	baseTs := google.TimeFromProto(desc.Timestamp)
	tags := assembleTags(desc.Tags)
	for _, blob := range r.Values {
		// TODO(vadimsh): LogEntry is already available in deserialized form few stack
		// frames up (in h.be.Logs in processLogStream).
		le := logpb.LogEntry{}
		if err := proto.Unmarshal(blob, &le); err != nil {
			logging.WithError(err).Errorf(ctx, "Failed to deserialized LogEntry")
			continue
		}
		out = append(out, rowToExport{
			Project:     string(r.Project),
			Path:        string(r.Path),
			Prefix:      desc.Prefix,
			Name:        desc.Name,
			Tags:        tags,
			Timestamp:   baseTs.Add(google.DurationFromProto(le.TimeOffset)),
			StreamIndex: int64(le.StreamIndex),
			PrefixIndex: int64(le.PrefixIndex),
			Lines:       assembleLines(le.GetText()),
		})
	}
	return out
}

func assembleTags(tags map[string]string) []string {
	out := make([]string, 0, len(tags))
	for k, v := range tags {
		out = append(out, k+":"+v)
	}
	sort.Strings(out)
	return out
}

func assembleLines(text *logpb.Text) []string {
	if text == nil {
		return nil
	}
	out := make([]string, len(text.Lines))
	for i := range text.Lines {
		out[i] = text.Lines[i].Value
	}
	return out
}
