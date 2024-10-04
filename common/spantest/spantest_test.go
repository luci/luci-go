// Copyright 2024 The LUCI Authors.
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

package spantest

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"
)

func TestMain(m *testing.M) {
	SpannerTestMain(m, "testdata/init_db.sql")
}

func TestSpantest(t *testing.T) {
	ctx := SpannerTestContext(t, nil)

	_, err := span.Apply(ctx, []*spanner.Mutation{spanner.Insert("TestTable",
		[]string{"ID"},
		[]any{"Hello"},
	)})
	if err != nil {
		t.Fatal(err)
	}

	tctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	var read []string
	err = span.Query(tctx, spanner.NewStatement("SELECT * FROM TestTable;")).Do(func(r *spanner.Row) error {
		var id string
		if err := r.Column(0, &id); err != nil {
			return err
		}
		read = append(read, id)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(read) != 1 || read[0] != "Hello" {
		t.Fatalf("Got %q", read)
	}
}
