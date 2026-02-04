// Copyright 2026 The LUCI Authors.
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

package data

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/id"
)

// DataSet represents a collection of TurboCI Datum messages keyed by their
// type.
//
// Must be constructed via one of:
//   - [CheckOptionDataSetFromSlice]
//   - [CheckResultDataSetFromSlice]
//   - [CheckEditOptionDataSetFromSlice]
//
// Zero value has undefined behavior.
type DataSet struct {
	data map[string]*orchestratorpb.Datum

	getIdx func(*idspb.Identifier) int
	mkId   func(int) *idspb.Identifier
}

// Get retrieves the value from the set (if it exists) or nil otherwise.
func (d *DataSet) Get(typeUrl string) *orchestratorpb.Datum {
	return d.data[typeUrl]
}

// Set sets the given value in this set, overriding any existing value of the
// same type.
//
// If `rv` introduces a new type to this DataSet, it must include Realm.
// If `rv` specifies realm for an existing type, the realm must match.
func (d *DataSet) Set(rv *orchestratorpb.WriteNodesRequest_RealmValue) error {
	realm := rv.GetRealm()
	key := rv.GetValue().GetValue().GetTypeUrl()
	cur, ok := d.data[key]
	if !ok {
		if realm == "" {
			return fmt.Errorf("adding type %q: no realm set", key)
		}
		d.data[key] = orchestratorpb.Datum_builder{
			Identifier: d.mkId(len(d.data) + 1),
			Realm:      &realm,
			Value:      rv.GetValue(),
			Version:    &orchestratorpb.Revision{},
		}.Build()
		return nil
	}

	if realm != "" && realm != cur.GetRealm() {
		return fmt.Errorf("setting type %q: mismatched realm: got %q want %q",
			key, rv.GetRealm(), cur.GetRealm())
	}

	cur.SetValue(rv.GetValue())
	cur.SetVersion(&orchestratorpb.Revision{})
	return nil
}

// Add inserts the given value into this set, but only if it does not already
// exist.
//
// Returns `true` if the value was inserted, `false` if it was already present.
//
// Returns error if `rv` is malformed (e.g. no Realm, and would require an
// insert).
func (d *DataSet) Add(rv *orchestratorpb.WriteNodesRequest_RealmValue) (bool, error) {
	if _, ok := d.data[rv.GetValue().GetValue().GetTypeUrl()]; ok {
		return false, nil
	}
	return true, d.Set(rv)
}

// Redact removes all data (value.value and value_json), from this DataSet,
// setting the omitted reason as `NO_ACCESS`, for all Datum objects where
// `pred` returns `true`.
//
// This retains only the type_urls of the redacted data.
func (d *DataSet) Redact(pred func(realm, typeURL string) bool) {
	for _, dat := range d.data {
		if pred(dat.GetRealm(), dat.GetValue().GetValue().GetTypeUrl()) {
			Redact(dat.GetValue())
		}
	}
}

// Filter will:
//   - Remove data from unwanted types (setting the omit reason to `UNWANTED`).
//   - Replace value data for types where JSONPB encoding is desired (and set
//     has_unknown_fields if this process could not fully reserialize the
//     value).
//
// If set, `mopt.Resolver` will be used for decoding and encoding all data
// where JSONPB encoding is desired. If there is no type available to decode
// in the resolver, this will leave the binary encoded data as-is.
func (d *DataSet) Filter(ti *TypeInfo, mopt protojson.MarshalOptions) error {
	var errs []error
	for _, dat := range d.data {
		if err := Filter(dat.GetValue(), ti, mopt); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ToSlice returns the values in the set, sorted by identifier idx.
func (d *DataSet) ToSlice() []*orchestratorpb.Datum {
	ret := make([]*orchestratorpb.Datum, 0, len(d.data))
	for _, dat := range d.data {
		ret = append(ret, dat)
	}
	slices.SortFunc(ret, func(a, b *orchestratorpb.Datum) int {
		return d.getIdx(a.GetIdentifier()) - d.getIdx(b.GetIdentifier())
	})
	return ret
}

func dataSetFromSlice(data []*orchestratorpb.Datum, wantKind idspb.IdentifierKind, getIdx func(*idspb.Identifier) int, mkId func(int) *idspb.Identifier) (*DataSet, error) {
	ret := make(map[string]*orchestratorpb.Datum, len(data))
	for i, dat := range data {
		ident := dat.GetIdentifier()
		if ident == nil {
			return nil, fmt.Errorf("bug: existing datum slice[%d] has nil identifier", i)
		}
		key := dat.GetValue().GetValue().GetTypeUrl()
		if !strings.HasPrefix(key, TypePrefix) {
			return nil, fmt.Errorf("datum slice[%d] type_url: does not start with %q (got %q)", i, TypePrefix, key)
		}
		if gotKind := id.KindOf(ident); gotKind != wantKind {
			return nil, fmt.Errorf("datum slice[%d] id kind mismatch: want=%q got=%q", i, wantKind, gotKind)
		}
		if _, ok := ret[key]; ok {
			return nil, fmt.Errorf("datum slice[%d]: duplicate kind %q", i, key)
		}
		ret[key] = dat
	}
	return &DataSet{ret, getIdx, mkId}, nil
}

// CheckOptionDataSetFromSlice makes a DataSet for composing Check.options.
//
// Takes possession of `chkID`; don't mutate it.
func CheckOptionDataSetFromSlice(chkID *idspb.Check, data []*orchestratorpb.Datum) (*DataSet, error) {
	return dataSetFromSlice(
		data,
		idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_OPTION,
		func(i *idspb.Identifier) int {
			return int(i.GetCheckOption().GetIdx())
		},
		func(i int) *idspb.Identifier {
			return id.Wrap(idspb.CheckOption_builder{
				Check: chkID,
				Idx:   proto.Int32(int32(i)),
			}.Build())
		},
	)
}

// CheckResultDataSetFromSlice makes a DataSet for composing Check.results.data.
//
// Takes possession of `rsltID`; don't mutate it.
func CheckResultDataSetFromSlice(rsltID *idspb.CheckResult, data []*orchestratorpb.Datum) (*DataSet, error) {
	return dataSetFromSlice(
		data,
		idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_RESULT_DATUM,
		func(i *idspb.Identifier) int {
			return int(i.GetCheckResultDatum().GetIdx())
		},
		func(i int) *idspb.Identifier {
			return id.Wrap(idspb.CheckResultDatum_builder{
				Result: rsltID,
				Idx:    proto.Int32(int32(i)),
			}.Build())
		},
	)
}

// CheckEditOptionDataSetFromSlice makes a DataSet for composing Check Edit.options.
//
// Takes possession of `editID`; don't mutate it.
func CheckEditOptionDataSetFromSlice(editID *idspb.CheckEdit, data []*orchestratorpb.Datum) (*DataSet, error) {
	return dataSetFromSlice(
		data,
		idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_EDIT_OPTION,
		func(i *idspb.Identifier) int {
			return int(i.GetCheckEditOption().GetIdx())
		},
		func(i int) *idspb.Identifier {
			return id.Wrap(idspb.CheckEditOption_builder{
				CheckEdit: editID,
				Idx:       proto.Int32(int32(i)),
			}.Build())
		},
	)
}
