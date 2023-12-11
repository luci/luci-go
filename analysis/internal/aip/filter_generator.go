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

package aip

import (
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"

	spanutil "go.chromium.org/luci/analysis/internal/span"
)

// whereClause constructs Standard SQL WHERE clause parts from
// column definitions and a parsed AIP-160 filter.
type whereClause struct {
	table         *Table
	parameters    []QueryParameter
	namePrefix    string
	nextValueName int
}

// QueryParameter represents a query parameter.
type QueryParameter struct {
	Name  string
	Value string
}

// WhereClause creates a Standard SQL WHERE clause fragment for the given filter.
//
// The fragment will be enclosed in parentheses and does not include the "WHERE" keyword.
// For example: (column LIKE @param1)
// Also returns the query parameters which need to be given to the database.
//
// All field names are replaced with the safe database column names from the specified table.
// All user input strings are passed via query parameters, so the returned query is SQL injection safe.
func (t *Table) WhereClause(filter *Filter, parameterPrefix string) (string, []QueryParameter, error) {
	if filter.Expression == nil {
		return "(TRUE)", []QueryParameter{}, nil
	}

	q := &whereClause{
		table:      t,
		namePrefix: parameterPrefix,
	}

	clause, err := q.expressionQuery(filter.Expression)
	if err != nil {
		return "", []QueryParameter{}, err
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
		if len(restriction.Comparable.Member.Fields) > 0 {
			value := restriction.Comparable.Member.Value
			fields := strings.Join(restriction.Comparable.Member.Fields, ".")
			return "", fmt.Errorf("fields are not allowed without an operator, try wrapping %s.%s in double quotes: \"%s.%s\"", value, fields, value, fields)
		}
		arg, err := w.likeComparableValue(restriction.Comparable)
		if err != nil {
			return "", err
		}
		clauses := []string{}
		// This is a value that should be substring matched against columns
		// marked for implicit matching.
		for _, column := range w.table.columns {
			if column.implicitFilter {
				clauses = append(clauses, fmt.Sprintf("%s LIKE %s", column.databaseName, arg))
			}
		}
		return "(" + strings.Join(clauses, " OR ") + ")", nil
	}
	column, err := w.table.FilterableColumnByFieldPath(NewFieldPath(restriction.Comparable.Member.Value))
	if err != nil {
		return "", err
	}
	if len(restriction.Comparable.Member.Fields) > 0 {
		if !column.keyValue {
			return "", fmt.Errorf("fields are only supported for key value columns.  Try removing the '.' from after your column named %q", column.fieldPath.String())
		}
		if len(restriction.Comparable.Member.Fields) > 1 {
			return "", fmt.Errorf("expected only a single '.' in keyvalue column named %q", column.fieldPath.String())
		}
		key := w.bind(restriction.Comparable.Member.Fields[0])
		if restriction.Comparator == ":" {
			value, err := w.likeArgValue(restriction.Arg, column)
			if err != nil {
				return "", errors.Annotate(err, "argument for field %s", column.fieldPath.String()).Err()
			}
			return fmt.Sprintf("(EXISTS (SELECT key, value FROM UNNEST(%s) WHERE key = %s AND value LIKE %s))", column.databaseName, key, value), nil
		}
		value, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", errors.Annotate(err, "argument for field %s", column.fieldPath.String()).Err()
		}
		if restriction.Comparator == "=" {
			return fmt.Sprintf("(EXISTS (SELECT key, value FROM UNNEST(%s) WHERE key = %s AND value = %s))", column.databaseName, key, value), nil
		} else if restriction.Comparator == "!=" {
			return fmt.Sprintf("(EXISTS (SELECT key, value FROM UNNEST(%s) WHERE key = %s AND value <> %s))", column.databaseName, key, value), nil
		}
		return "", fmt.Errorf("comparator operator not implemented for fields yet")
	} else if column.keyValue {
		// TODO: AIP-160 specifies the has operator on maps will check for the presence of a key.
		return "", fmt.Errorf("key value columns must specify the key to search on.  Instead of '%s%s' try '%s.key%s'", column.fieldPath.String(), restriction.Comparator, column.fieldPath.String(), restriction.Comparator)
	}
	if restriction.Comparator == "=" {
		arg, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", errors.Annotate(err, "argument for field %s", column.fieldPath.String()).Err()
		}
		return fmt.Sprintf("(%s = %s)", column.databaseName, arg), nil
	} else if restriction.Comparator == "!=" {
		arg, err := w.argValue(restriction.Arg, column)
		if err != nil {
			return "", errors.Annotate(err, "argument for field %s", column.fieldPath.String()).Err()
		}
		return fmt.Sprintf("(%s <> %s)", column.databaseName, arg), nil
	} else if restriction.Comparator == ":" {
		arg, err := w.likeArgValue(restriction.Arg, column)
		if err != nil {
			return "", errors.Annotate(err, "argument for field %s", column.fieldPath.String()).Err()
		}
		return fmt.Sprintf("(%s LIKE %s)", column.databaseName, arg), nil
	} else {
		return "", fmt.Errorf("comparator operator not implemented yet")
	}
}

// argValue returns a SQL expression representing the value of the specified
// arg.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) argValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions in arguments not implemented yet")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	return w.comparableValue(arg.Comparable, column)
}

// argValue returns a SQL expression representing the value of the specified
// comparable.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) comparableValue(comparable *Comparable, column *Column) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields not implemented yet")
	}
	switch column.columnType {
	case ColumnTypeString:
		value := comparable.Member.Value
		if column.argSubstitute != nil {
			value = column.argSubstitute(value)
		}
		// Bind unsanitised user input to a parameter to protect against SQL injection.
		return w.bind(value), nil
	case ColumnTypeBool:
		if strings.EqualFold(comparable.Member.Value, "true") {
			return "TRUE", nil
		} else if strings.EqualFold(comparable.Member.Value, "false") {
			return "FALSE", nil
		}
		return "", fmt.Errorf("only TRUE or FALSE can be specified as the value for a boolean field")
	}
	return "", fmt.Errorf("unable to generate SQL value for unknown field type: %s", column.columnType.String())
}

// likeArgValue returns a SQL expression that, when passed to the
// right hand side of a LIKE operator, performs substring matching against
// the value of the argument.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) likeArgValue(arg *Arg, column *Column) (string, error) {
	if arg.Composite != nil {
		return "", fmt.Errorf("composite expressions are not allowed as RHS to has (:) operator")
	}
	if arg.Comparable == nil {
		return "", fmt.Errorf("missing comparable in argument")
	}
	if column.columnType != ColumnTypeString {
		return "", fmt.Errorf("cannot use has (:) operator on a non-string field %q", column.columnType.String())
	}
	if column.argSubstitute != nil {
		return "", fmt.Errorf("cannot use has (:) operator on a field that have argSubstitute function")
	}
	return w.likeComparableValue(arg.Comparable)
}

// likeComparableValue returns a SQL expression that, when passed to the
// right hand side of a LIKE operator, performs substring matching against
// the value of the comparable.
// The returned string is an injection-safe SQL expression.
func (w *whereClause) likeComparableValue(comparable *Comparable) (string, error) {
	if comparable.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	if len(comparable.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed on the RHS of has (:) operator")
	}
	// Bind unsanitised user input to a parameter to protect against SQL injection.
	return w.bind("%" + spanutil.QuoteLike(comparable.Member.Value) + "%"), nil
}

// bind binds a new query parameter with the given value, and returns
// the name of the parameter (including '@').
// The returned string is an injection-safe SQL expression.
func (w *whereClause) bind(value string) string {
	name := w.namePrefix + strconv.Itoa(w.nextValueName)
	w.nextValueName += 1
	w.parameters = append(w.parameters, QueryParameter{Name: name, Value: value})
	return "@" + name
}
