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

package main

import (
	"testing"

	"cloud.google.com/go/bigquery"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAddMissingFields(t *testing.T) {
	Convey("Rename field", t, func() {
		from := bigquery.Schema{
			{
				Name: "a",
				Type: bigquery.IntegerFieldType,
			},
			{
				Name: "b",
				Type: bigquery.RecordFieldType,
				Schema: bigquery.Schema{
					{
						Name: "c",
						Type: bigquery.IntegerFieldType,
					},
					{
						Name: "d",
						Type: bigquery.IntegerFieldType,
					},
				},
			},
		}
		to := bigquery.Schema{
			{
				Name: "e",
				Type: bigquery.IntegerFieldType,
			},
			{
				Name: "b",
				Type: bigquery.RecordFieldType,
				Schema: bigquery.Schema{
					{
						Name: "c",
						Type: bigquery.IntegerFieldType,
					},
				},
			},
		}
		addMissingFields(&to, from)
		So(to, ShouldResemble, bigquery.Schema{
			{
				Name: "e",
				Type: bigquery.IntegerFieldType,
			},
			{
				Name: "b",
				Type: bigquery.RecordFieldType,
				Schema: bigquery.Schema{
					{
						Name: "c",
						Type: bigquery.IntegerFieldType,
					},
					{
						Name: "d",
						Type: bigquery.IntegerFieldType,
					},
				},
			},
			{
				Name: "a",
				Type: bigquery.IntegerFieldType,
			},
		})
	})
}
