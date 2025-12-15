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

package aip160

import (
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/data/aip132"
)

// whereClause constructs Standard SQL WHERE clause parts from
// column definitions and a parsed AIP-160 filter.
type whereClause struct {
	table      *DatabaseTable
	parameters []SqlQueryParameter
	// The prefix to apply to generated SQL parameter names. Used to deconflict
	// filter parameters from other parameters.
	parameterPrefix string
	// The table alias (if any) to use when generating SQL column names.
	// Useful if the table has an alias (e.g. because there is a JOIN).
	tableAlias    string
	nextValueName int
}

// SqlQueryParameter represents a query parameter.
type SqlQueryParameter struct {
	Name  string
	Value string
}

// WhereClause creates a Google Standard SQL WHERE clause fragment for the given filter.
//
// The fragment will be enclosed in parentheses and does not include the "WHERE" keyword.
// For example: (column LIKE @param1)
// Also returns the query parameters which need to be given to the database.
//
// All field names are replaced with the safe database column names from the specified table.
// All user input strings are passed via query parameters, so the returned query is SQL injection safe.
func (t *DatabaseTable) WhereClause(filter *Filter, tableAlias, parameterPrefix string) (string, []SqlQueryParameter, error) {
	if filter == nil || filter.Expression == nil {
		return "(TRUE)", []SqlQueryParameter{}, nil
	}
	if strings.HasPrefix(tableAlias, "_") {
		// Generated SQL may use table aliases, e.g. for UNNEST clauses.
		// We reserve table aliases starting with '_' for use within generated SQL,
		// to avoid bugs.
		return "", []SqlQueryParameter{}, fmt.Errorf("table aliases starting with '_' are reserved for use within generated SQL")
	}

	q := &whereClause{
		table:           t,
		tableAlias:      tableAlias,
		parameterPrefix: parameterPrefix,
	}

	clause, err := q.expressionQuery(filter.Expression)
	if err != nil {
		return "", []SqlQueryParameter{}, err
	}
	return clause, q.parameters, nil
}

// expressionQuery returns the SQL expression equivalent to the given
// filter expression.
// An expression is a conjunction (AND) of sequences or a simple
// sequence.
//
// The returned string is an injection-safe SQL expression.
func (w *whereClause) expressionQuery(expression *Expression) (string, error) {
	factors := []string{}
	// Both Sequence and Factor is equivalent to AND of the
	// component Sequences and Factors (respectively), as we implement
	// exact match semantics and do not support ranking
	// based on the number of factors that match.
	for _, sequence := range expression.Sequences {
		for _, factor := range sequence.Factors {
			f, err := w.factorQuery(factor)
			if err != nil {
				return "", err
			}
			factors = append(factors, f)
		}
	}
	if len(factors) == 1 {
		return factors[0], nil
	}
	return "(" + strings.Join(factors, " AND ") + ")", nil
}

// factorQuery returns the SQL expression equivalent to the given
// factor. A factor is a disjunction (OR) of terms or a simple term.
//
// The returned string is an injection-safe SQL expression.
func (w *whereClause) factorQuery(factor *Factor) (string, error) {
	terms := []string{}
	for _, term := range factor.Terms {
		tq, err := w.termQuery(term)
		if err != nil {
			return "", err
		}
		terms = append(terms, tq)
	}
	if len(terms) == 1 {
		return terms[0], nil
	}
	return "(" + strings.Join(terms, " OR ") + ")", nil
}

// termQuery returns the SQL expression equivalent to the given
// term.
//
// The returned string is an injection-safe SQL expression.
func (w *whereClause) termQuery(term *Term) (string, error) {
	simpleQuery, err := w.simpleQuery(term.Simple)
	if err != nil {
		return "", err
	}
	if term.Negated {
		return fmt.Sprintf("(NOT %s)", simpleQuery), nil
	}
	return simpleQuery, nil
}

// simpleQuery returns the SQL expression equivalent to the given simple
// filter.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) simpleQuery(simple *Simple) (string, error) {
	if simple.Restriction != nil {
		return w.restrictionQuery(simple.Restriction)
	} else if simple.Composite != nil {
		return w.expressionQuery(simple.Composite)
	} else {
		return "", fmt.Errorf("invalid 'simple' clause in query filter")
	}
}

// restrictionQuery returns the SQL expression equivalent to the given
// restriction.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) restrictionQuery(restriction *Restriction) (string, error) {
	if restriction.Comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if restriction.Comparator == "" {
		arg, err := coerceComparableToImplicitFilter(restriction.Comparable)
		if err != nil {
			return "", err
		}
		clauses := []string{}
		context := ImplicitRestrictionContext{
			ArgValueUnsafe: arg,
		}
		// This is a value that should be substring matched against columns
		// marked for implicit matching.
		for _, column := range w.table.fields {
			if column.implicitFilter {
				clause, err := column.backend.ImplicitRestrictionQuery(context, w)
				if err != nil {
					return "", fmt.Errorf("implicit restriction on field %s: %w", column.fieldPath.String(), err)
				}
				clauses = append(clauses, clause)
			}
		}
		return "(" + strings.Join(clauses, " OR ") + ")", nil
	}
	if restriction.Comparable.Member.Value.Quoted {
		return "", fmt.Errorf("expected a field name on the left hand side of a restriction, got the string literal %q", restriction.Comparable.Member.Value)
	}
	column, err := w.table.FilterableFieldByFieldPath(aip132.NewFieldPath(restriction.Comparable.Member.Value.Value))
	if err != nil {
		return "", err
	}
	var nestedFields []string
	for _, fld := range restriction.Comparable.Member.Fields {
		nestedFields = append(nestedFields, fld.Value)
	}

	context := RestrictionContext{
		FieldPath:    column.fieldPath,
		NestedFields: nestedFields,
		Comparator:   restriction.Comparator,
		Arg:          restriction.Arg,
	}
	return column.backend.RestrictionQuery(context, w)
}

// quoteLike turns a literal string into an escaped like expression.
// This means strings like test_name will only match as expected, rather than
// also matching test3name.
func quoteLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	value = strings.ReplaceAll(value, "_", "\\_")
	return value
}

// BindString binds a new query parameter with the given string value, and returns
// the name of the parameter (including '@').
// The returned string is an injection-safe SQL expression.
func (w *whereClause) BindString(value string) string {
	name := w.parameterPrefix + strconv.Itoa(w.nextValueName)
	w.nextValueName += 1
	w.parameters = append(w.parameters, SqlQueryParameter{Name: name, Value: value})
	return "@" + name
}

// ColumnReference prepends the table alias (if any) to the given column name
// to provide the fully-qualified column name.
func (w *whereClause) ColumnReference(databaseName string) string {
	if w.tableAlias != "" {
		return w.tableAlias + "." + databaseName
	}
	return databaseName
}
