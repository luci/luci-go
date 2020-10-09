// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"fmt"
	"io"
	"strings"

	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/resultdb/pbutil"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

type printer struct {
	indented.Writer
	err error
}

func newPrinter(w io.Writer) *printer {
	return &printer{
		Writer: indented.Writer{
			Writer:    w,
			UseSpaces: true,
			Width:     2,
		},
	}
}

func (p *printer) printf(format string, args ...interface{}) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(&p.Writer, format, args...)
	}
}

func variantString(v map[string]string) string {
	pairs := pbutil.VariantToStrings(&resultpb.Variant{Def: v})
	return strings.Join(pairs, " | ")
}
