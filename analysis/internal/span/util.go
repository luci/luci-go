// Copyright 2022 The LUCI Authors.
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

package span

import (
	"bytes"
	"strings"
	"text/template"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
)

// GenerateStatement generates a spanner statement from a text template.
func GenerateStatement(tmpl *template.Template, name string, input any) (spanner.Statement, error) {
	sql := &bytes.Buffer{}
	err := tmpl.ExecuteTemplate(sql, name, input)
	if err != nil {
		return spanner.Statement{}, errors.Annotate(err, "failed to generate statement: %s", name).Err()
	}
	return spanner.NewStatement(sql.String()), nil
}

// QuoteLike turns a literal string into an escaped like expression.
// This means strings like test_name will only match as expected, rather than
// also matching test3name.
func QuoteLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	value = strings.ReplaceAll(value, "_", "\\_")
	return value
}
