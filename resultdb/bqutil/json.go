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

package bqutil

import (
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

// MarshalStructPB serialises a structpb.Struct as a JSONPB.
func MarshalStructPB(s *structpb.Struct) (string, error) {
	if s == nil {
		// There is no string value we can send to BigQuery that will
		// interpret as a NULL value for a JSON column:
		// - "" (empty string) is rejected as invalid JSON.
		// - "null" is interpreted as the JSON value null, not the
		//   absence of a value.
		// Consequently, the next best thing is to return an empty
		// JSON object, aka an empty Struct
		s, _ = structpb.NewStruct(map[string]interface{}{})
	}
	// Structs are persisted as JSONPB strings.
	// See also https://bit.ly/chromium-bq-struct
	b, err := (&protojson.MarshalOptions{}).Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// MarshalStringStructPBMap serialises a map[string]*structpb.Struct as a JSONPB.
func MarshalStringStructPBMap(m map[string]*structpb.Struct) (string, error) {
	if m == nil {
		// There is no string value we can send to BigQuery that will
		// interpret as a NULL value for a JSON column:
		// - "" (empty string) is rejected as invalid JSON.
		// - "null" is interpreted as the JSON value null, not the
		//   absence of a value.
		// Consequently, the next best thing is to return an empty
		// JSON object, aka an empty map
		//m, _ = structpb.NewStruct(map[string]interface{}{})
		m = map[string]*structpb.Struct{}
	}
	// Convert map[string]*structpb.Struct into a nested map[string]interface{}
	i := map[string]interface{}{}
	for key, val := range m {
		i[key] = val.AsMap()
	}
	s, err := structpb.NewStruct(i)
	if err != nil {
		return "", err
	}
	// Structs are persisted as JSONPB strings.
	// See also https://bit.ly/chromium-bq-struct
	b, err := (&protojson.MarshalOptions{}).Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
