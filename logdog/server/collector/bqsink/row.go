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
	"fmt"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/storage"
)

// rowToExport defines the structure of bigquery row we export.
type rowToExport struct {
	Project string `bigquery:"project"`
	Path    string `bigquery:"path"`
	Index   int64  `bigquery:"index"`
	Text    string `bigquery:"text"`
}

func (r *rowToExport) insertID() string {
	return fmt.Sprintf("%s:%s:%d", r.Project, r.Path, r.Index)
}

var rowToExportSchema bigquery.Schema

func init() {
	var err error
	rowToExportSchema, err = bigquery.InferSchema(rowToExport{})
	if err != nil {
		panic(err)
	}
}

func toStuctSavers(rows []rowToExport) []bigquery.StructSaver {
	out := make([]bigquery.StructSaver, len(rows))
	for i := range rows {
		out[i] = bigquery.StructSaver{
			Schema:   rowToExportSchema,
			InsertID: rows[i].insertID(),
			Struct:   &rows[i],
		}
	}
	return out
}

func logsToRows(desc *logpb.LogStreamDescriptor, r *storage.PutRequest) []rowToExport {
	out := make([]rowToExport, len(r.Values))
	for i, blob := range r.Values {
		out[i] = rowToExport{
			Project: string(r.Project),
			Path:    string(r.Path),
			Index:   int64(r.Index) + int64(i),
			Text:    extractPlainText(blob),
		}
	}
	return out
}

func extractPlainText(blob []byte) string {
	return "" // TODO
}
