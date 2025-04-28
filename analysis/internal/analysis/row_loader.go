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

package analysis

import (
	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/errors"
)

// rowLoader provides a way of marshalling a BigQuery row.
// The intended usage is as follows (where it is the bigquery iterator):
//
// var loader rowLoader
// err := it.Next(&loader) // 'it' is a bigquery iterator.
// ... // handle err
// someField := loader.NullString("some_field")
// otherField := loader.Int64("other_field")
//
// if err := loader.Error(); err != nil {
// ... // handle err
// }
type rowLoader struct {
	vals   []bigquery.Value
	schema bigquery.Schema
	err    error
}

func (r *rowLoader) Load(v []bigquery.Value, s bigquery.Schema) error {
	r.vals = v
	r.schema = s
	r.err = nil
	return nil
}

func (r *rowLoader) fieldIndex(fieldName string) (index int, ok bool) {
	for i, field := range r.schema {
		if field.Name == fieldName {
			return i, true
		}
	}
	return -1, false
}

func (r *rowLoader) valueWithType(fieldName string, expectedType bigquery.FieldType, repeated bool) (bigquery.Value, error) {
	i, ok := r.fieldIndex(fieldName)
	if !ok {
		return nil, errors.Reason("field %s is not defined", fieldName).Err()
	}
	fieldType := r.schema[i]
	if fieldType.Type != expectedType {
		return nil, errors.Reason("field %s has type %s, expected type %s", fieldName, fieldType.Type, expectedType).Err()
	}
	if fieldType.Repeated != repeated {
		return nil, errors.Reason("field %s repeated=%v, expected repeated=%v", fieldName, fieldType.Repeated, repeated).Err()
	}
	return r.vals[i], nil
}

// NullString returns the value of a field of type bigquery.NullString.
// If the field does not exist or is of an incorrect type, the default
// bigquery.NullString is returned and an error will be available from
// rowLoader.Error().
func (r *rowLoader) NullString(fieldName string) bigquery.NullString {
	repeated := false
	val, err := r.valueWithType(fieldName, bigquery.StringFieldType, repeated)
	if err != nil {
		r.reportError(err)
		return bigquery.NullString{}
	}
	if val == nil {
		return bigquery.NullString{}
	}
	return bigquery.NullString{Valid: true, StringVal: val.(string)}
}

// String returns the value of a field of type string.
// If the field does not exist or is of an incorrect type, an empty
// string is returned and an error will be available from rowLoader.Error().
func (r *rowLoader) String(fieldName string) string {
	val := r.NullString(fieldName)
	if !val.Valid {
		r.reportError(errors.Reason("field %s value is NULL, expected non-null string", fieldName).Err())
		return ""
	}
	return val.String()
}

// NullInt64 returns the value of a field of type NullInt64.
// If the field does not exist or is of an incorrect type, the default
// NullInt64 is returned and an error will be available from rowLoader.Error().
func (r *rowLoader) NullInt64(fieldName string) bigquery.NullInt64 {
	repeated := false
	val, err := r.valueWithType(fieldName, bigquery.IntegerFieldType, repeated)
	if err != nil {
		r.reportError(err)
		return bigquery.NullInt64{}
	}
	if val == nil {
		return bigquery.NullInt64{}
	}
	return bigquery.NullInt64{Valid: true, Int64: val.(int64)}
}

// Int64 returns the value of a field of type Int64.
// If the field does not exist or is of an incorrect type, the value
// -1 is returned and an error will be available from rowLoader.Error().
func (r *rowLoader) Int64(fieldName string) int64 {
	val := r.NullInt64(fieldName)
	if !val.Valid {
		r.reportError(errors.Reason("field %s value is NULL, expected non-null integer", fieldName).Err())
		return -1
	}
	return val.Int64
}

// Int64s returns the value of a field of type []Int64.
// If the field does not exist or is of an incorrect type, an empty
// array is returned and an error will be available from rowLoader.Error().
func (r *rowLoader) Int64s(fieldName string) []int64 {
	repeated := true
	val, err := r.valueWithType(fieldName, bigquery.IntegerFieldType, repeated)
	if err != nil {
		r.reportError(err)
		return nil
	}
	rows := val.([]bigquery.Value)
	result := make([]int64, 0, len(rows))
	for i, row := range rows {
		if row == nil {
			r.reportError(errors.Reason("field %s index %v is NULL, expected non-null integer", fieldName, i).Err())
			return nil
		}
		result = append(result, row.(int64))
	}
	return result
}

// TopCounts returns the value of a field of type []TopCount.
// If the field does not exist or is of an incorrect type, an empty
// array is returned and an error will be available from rowLoader.Error().
func (r *rowLoader) TopCounts(fieldName string) []TopCount {
	repeated := true
	val, err := r.valueWithType(fieldName, bigquery.RecordFieldType, repeated)
	if err != nil {
		r.reportError(err)
		return nil
	}

	i, _ := r.fieldIndex(fieldName)
	rowSchema := r.schema[i].Schema

	var results []TopCount
	rows := val.([]bigquery.Value)
	for _, row := range rows {
		var nestedLoader rowLoader
		if err := nestedLoader.Load(row.([]bigquery.Value), rowSchema); err != nil {
			r.reportError(errors.Annotate(err, "field %s", fieldName).Err())
			return nil
		}

		var tc TopCount
		tc.Value = nestedLoader.String("value")
		tc.Count = nestedLoader.Int64("count")
		results = append(results, tc)

		if err := nestedLoader.Error(); err != nil {
			r.reportError(errors.Annotate(err, "field %s", fieldName).Err())
			return nil
		}
	}
	return results
}

// Strings returns the value of a field of type []string.
// If the field does not exist or is of an incorrect type, an empty
// array is returned and an error will be available from rowLoader.Error().
func (r *rowLoader) Strings(fieldName string) []string {
	repeated := true
	val, err := r.valueWithType(fieldName, bigquery.StringFieldType, repeated)
	if err != nil {
		r.reportError(err)
		return nil
	}
	rows := val.([]bigquery.Value)
	result := make([]string, 0, len(rows))
	for i, row := range rows {
		if row == nil {
			r.reportError(errors.Reason("field %s index %v is NULL, expected non-null string", fieldName, i).Err())
			return nil
		}
		result = append(result, row.(string))
	}
	return result
}

func (r *rowLoader) reportError(err error) {
	// Keep the first error that was reported.
	if r.err == nil {
		r.err = err
	}
}

// Error returns the first error that occured while marshalling the
// row (if any). It is exposed here to avoid needing boilerplate error
// handling code around every field marshalling operation.
func (r *rowLoader) Error() error {
	return r.err
}
