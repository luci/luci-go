// Copyright 2019 The LUCI Authors.
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

package tsmon

import (
	"fmt"
	"html/template"
	"sort"
	"strings"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/types"
)

// formatCellsAsHTML returns an HTML fragment with data from `cells`.
//
// Mutates order of `cells` as a side effect.
//
// Caveats:
//   - Totally ignores cell.Target for now.
//   - Ignores cell.Units, it is always empty for some reason.
//   - Distributions are replaced with the average value they hold.
func formatCellsAsHTML(cells []types.Cell) template.HTML {
	// Sort by the metric name, then by field values.
	sort.Slice(cells, func(i, j int) bool {
		switch l, r := cells[i], cells[j]; {
		case l.Name != r.Name:
			return l.Name < r.Name
		case len(l.FieldVals) != len(r.FieldVals): // this should not be happening
			return len(l.FieldVals) < len(r.FieldVals)
		default:
			for idx := range l.FieldVals {
				lstr := fmt.Sprintf("%v", l.FieldVals[idx])
				rstr := fmt.Sprintf("%v", r.FieldVals[idx])
				if lstr != rstr {
					return lstr < rstr
				}
			}
			return false
		}
	})

	buf := htmlBuilder{}
	buf.styles()

	// Shorter aliases to unclutter the code.
	table := buf.table
	tr := buf.tr
	td := buf.td
	bold := buf.bold
	metric := buf.metric
	value := buf.value

	// First render a table with "singleton" metrics: ones without any fields at
	// all (usually something global to the process).
	table(func() {
		for _, c := range cells {
			if len(c.Fields) == 0 {
				tr(func() {
					td(func() { metric("b", c.Name, c.ValueType) })
					td(func() { value(c.Value) })
				})
			}
		}
	})
	buf.WriteString("<hr>")

	// For each metric that uses fields, render a separate table. Note that this
	// loop relies on cells being sorted by the metric name.
	for idx := 0; idx < len(cells); idx++ {
		if len(cells[idx].Fields) == 0 {
			continue
		}

		// Scan until we hit the cell from another metric.
		start := idx
		for idx < len(cells) && cells[idx].Name == cells[start].Name {
			idx++
		}
		display := cells[start:idx]
		idx-- // will be incremented again by the for loop

		metric("h4", display[0].Name, display[0].ValueType)
		table(func() {
			// A row with names of the fields.
			tr(func() {
				for _, f := range display[0].Fields {
					td(func() { bold(f.Name) })
				}
				td(func() { bold("value") })
			})
			// A row per combination of metrics.
			for _, c := range display {
				tr(func() {
					for _, v := range c.FieldVals {
						td(func() { value(v) })
					}
					td(func() { value(c.Value) })
				})
			}
		})
	}

	return template.HTML(buf.String())
}

// htmlBuilder is a helper to construct HTML tables with metrics.
//
// Using it is overall simpler and faster *in this case*, than using
// "template/html", since we can process the metrics and emit HTML in one pass.
//
// Using HTML templates requires to prepare data for tables beforehand (in a
// bunch of structs and slices), and only then render it. This is justifiable if
// we expect frequently changes to how the data is displayed (and so want to
// split the view into a standalone HTML template). But *in this case* we don't,
// so the artisanally crafted HTML is fine and helps us avoid unnecessary code.
type htmlBuilder struct {
	strings.Builder
}

func (b *htmlBuilder) styles() {
	b.WriteString(`
		<style>
			#metrics-table td { padding: 2px 5px 2px 5px; }
		</style>
	`)
}

func (b *htmlBuilder) table(cb func()) {
	b.WriteString(`<div class="small">`)
	b.WriteString(`<table id="metrics-table" class="table table-bordered">`)
	b.WriteRune('\n')
	cb()
	b.WriteString(`</table>`)
	b.WriteString(`</div>`)
	b.WriteRune('\n')
}

func (b *htmlBuilder) tr(cb func()) {
	b.WriteString("<tr>")
	cb()
	b.WriteString("</tr>\n")
}

func (b *htmlBuilder) td(cb func()) {
	b.WriteString("<td>")
	cb()
	b.WriteString("</td>")
}

func (b *htmlBuilder) bold(n string) {
	b.WriteString("<b>")
	template.HTMLEscape(b, []byte(n))
	b.WriteString("</b>")
}

func (b *htmlBuilder) metric(tag, name string, typ types.ValueType) {
	fmt.Fprintf(b, "<%s>", tag)
	template.HTMLEscape(b, []byte(name))
	if typ.IsCumulative() {
		b.WriteString(" <small>(cumulative)</small>")
	}
	fmt.Fprintf(b, "</%s>", tag)
}

func (b *htmlBuilder) value(v any) {
	if dis, ok := v.(*distribution.Distribution); ok {
		count := dis.Count()
		if count == 0 {
			b.WriteString("<i>empty distribution</i>")
		} else {
			fmt.Fprintf(b, "<b>avg:</b> %.2f", dis.Sum()/float64(count))
		}
	} else {
		template.HTMLEscape(b, []byte(fmt.Sprintf("%v", v)))
	}
}
