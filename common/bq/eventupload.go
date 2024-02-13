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

package bq

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

// ID is the global InsertIDGenerator
var ID InsertIDGenerator

const insertLimit = 10000
const batchDefault = 500

// Uploader contains the necessary data for streaming data to BigQuery.
type Uploader struct {
	*bigquery.Inserter
	// Uploader is bound to a specific table. DatasetID and Table ID are
	// provided for reference.
	DatasetID string
	TableID   string
	// UploadsMetricName is a string used to create a tsmon Counter metric
	// for event upload attempts via Put, e.g.
	// "/chrome/infra/commit_queue/events/count". If unset, no metric will
	// be created.
	UploadsMetricName string
	// uploads is the Counter metric described by UploadsMetricName. It
	// contains a field "status" set to either "success" or "failure."
	uploads        metric.Counter
	initMetricOnce sync.Once
	// BatchSize is the max number of rows to send to BigQuery at a time.
	// The default is 500.
	BatchSize int
}

// Row implements bigquery.ValueSaver
type Row struct {
	proto.Message // embedded

	// InsertID is unique per insert operation to handle deduplication.
	InsertID string
}

// Save is used by bigquery.Inserter.Put when inserting values into a table.
func (r *Row) Save() (map[string]bigquery.Value, string, error) {
	m, err := mapFromMessage(r.Message, nil)
	return m, r.InsertID, err
}

// mapFromMessage returns a {BQ Field name: BQ value} map.
// path is a slice of Go field names leading to m.
func mapFromMessage(pm proto.Message, path []string) (map[string]bigquery.Value, error) {
	type kvPair struct {
		key string
		val protoreflect.Value
	}

	m := pm.ProtoReflect()
	if !m.IsValid() {
		return nil, nil
	}
	fields := m.Descriptor().Fields()

	var row map[string]bigquery.Value // keep it nil unless there are values
	path = append(path, "")

	for i := 0; i < fields.Len(); i++ {
		var bqValue any
		var err error
		field := fields.Get(i)
		fieldValue := m.Get(field)
		bqField := string(field.Name())
		path[len(path)-1] = bqField

		switch {
		case field.IsList():
			list := fieldValue.List()

			elems := make([]any, 0, list.Len())
			vPath := append(path, "")
			for i := 0; i < list.Len(); i++ {
				vPath[len(vPath)-1] = strconv.Itoa(i)
				elemValue, err := getValue(field, list.Get(i), vPath)
				if err != nil {
					return nil, errors.Annotate(err, "%s[%d]", bqField, i).Err()
				}
				if elemValue == nil {
					continue
				}
				elems = append(elems, elemValue)
			}
			if len(elems) == 0 {
				continue
			}
			bqValue = elems
		case field.IsMap():
			if field.MapKey().Kind() != protoreflect.StringKind {
				return nil, fmt.Errorf("map key must be a string")
			}

			mapValue := fieldValue.Map()
			if mapValue.Len() == 0 {
				continue
			}

			pairs := make([]kvPair, 0, mapValue.Len())
			mapValue.Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
				pairs = append(pairs, kvPair{key.String(), value})
				return true
			})
			slices.SortFunc(pairs, func(i, j kvPair) int {
				switch {
				case i.key == j.key:
					return 0
				case i.key < j.key:
					return -1
				default:
					return 1
				}
			})

			valueDesc := field.MapValue()
			elems := make([]any, mapValue.Len())
			vPath := append(path, "")
			for i, pair := range pairs {
				vPath[len(vPath)-1] = pair.key
				elemValue, err := getValue(valueDesc, pair.val, vPath)
				if err != nil {
					return nil, errors.Annotate(err, "%s[%s]", bqField, pair.key).Err()
				}

				elems[i] = map[string]bigquery.Value{
					"key":   pair.key,
					"value": elemValue,
				}
			}

			bqValue = elems
		default:
			if bqValue, err = getValue(field, fieldValue, path); err != nil {
				return nil, errors.Annotate(err, "%s", bqField).Err()
			} else if bqValue == nil {
				// Omit NULL/nil values
				continue
			}
		}

		if row == nil {
			row = map[string]bigquery.Value{}
		}
		row[bqField] = bigquery.Value(bqValue)
	}

	return row, nil
}

func getValue(field protoreflect.FieldDescriptor, value protoreflect.Value, path []string) (any, error) {
	// enums and primitives
	if enumField := field.Enum(); enumField != nil {
		enumName := string(enumField.Values().ByNumber(value.Enum()).Name())
		return enumName, nil
	}
	if field.Kind() != protoreflect.MessageKind && field.Kind() != protoreflect.GroupKind {
		return value.Interface(), nil
	}

	// structs
	messageInterface := value.Message().Interface()
	if dpb, ok := messageInterface.(*durationpb.Duration); ok {
		if dpb == nil {
			return nil, nil
		}
		if err := dpb.CheckValid(); err != nil {
			return nil, fmt.Errorf("tried to write an invalid duration for [%+v] for field %q", dpb, strings.Join(path, "."))
		}
		value := dpb.AsDuration()
		// Convert to FLOAT64.
		return value.Seconds(), nil
	}
	if tspb, ok := messageInterface.(*timestamppb.Timestamp); ok {
		if tspb == nil {
			return nil, nil
		}
		if err := tspb.CheckValid(); err != nil {
			return nil, fmt.Errorf("tried to write an invalid timestamp for [%+v] for field %q", tspb, strings.Join(path, "."))
		}
		value := tspb.AsTime()
		return value, nil
	}
	if s, ok := messageInterface.(*structpb.Struct); ok {
		if s == nil {
			return nil, nil
		}
		// Structs are persisted as JSONPB strings.
		// See also https://bit.ly/chromium-bq-struct
		var buf []byte
		var err error
		if buf, err = protojson.Marshal(s); err != nil {
			return nil, err
		}
		return string(buf), nil
	}
	message, err := mapFromMessage(messageInterface, path)
	if message == nil {
		// a nil map is not nil when converted to any,
		// so return nil explicitly.
		return nil, err
	}
	return message, err
}

// NewUploader constructs a new Uploader struct.
//
// DatasetID and TableID are provided to the BigQuery client to
// gain access to a particular table.
//
// You may want to change the default configuration of the bigquery.Inserter.
// Check the documentation for more details.
//
// Set UploadsMetricName on the resulting Uploader to use the default counter
// metric.
//
// Set BatchSize to set a custom batch size.
func NewUploader(ctx context.Context, c *bigquery.Client, datasetID, tableID string) *Uploader {
	return &Uploader{
		DatasetID: datasetID,
		TableID:   tableID,
		Inserter:  c.Dataset(datasetID).Table(tableID).Inserter(),
	}
}

func (u *Uploader) batchSize() int {
	switch {
	case u.BatchSize > insertLimit:
		return insertLimit
	case u.BatchSize <= 0:
		return batchDefault
	default:
		return u.BatchSize
	}
}

func (u *Uploader) getCounter() metric.Counter {
	u.initMetricOnce.Do(func() {
		if u.UploadsMetricName != "" {
			desc := "Upload attempts; status is 'success' or 'failure'"
			field := field.String("status")
			u.uploads = metric.NewCounter(u.UploadsMetricName, desc, nil, field)
		}
	})
	return u.uploads
}

func (u *Uploader) updateUploads(ctx context.Context, count int64, status string) {
	if uploads := u.getCounter(); uploads != nil && count != 0 {
		uploads.Add(ctx, count, status)
	}
}

// Put uploads one or more rows to the BigQuery service. Put takes care of
// adding InsertIDs, used by BigQuery to deduplicate rows.
//
// If any rows do now match one of the expected types, Put will not attempt to
// upload any rows and returns an InvalidTypeError.
//
// Put returns a PutMultiError if one or more rows failed to be uploaded.
// The PutMultiError contains a RowInsertionError for each failed row.
//
// Put will retry on temporary errors. If the error persists, the call will
// run indefinitely. Because of this, if ctx does not have a timeout, Put will
// add one.
//
// See bigquery documentation and source code for detailed information on how
// struct values are mapped to rows.
func (u *Uploader) Put(ctx context.Context, messages ...proto.Message) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}
	rows := make([]*Row, len(messages))
	for i, m := range messages {
		rows[i] = &Row{
			Message:  m,
			InsertID: ID.Generate(),
		}
	}

	return parallel.WorkPool(16, func(workC chan<- func() error) {
		for _, rowSet := range batch(rows, u.batchSize()) {
			rowSet := rowSet
			workC <- func() error {
				var failed int
				err := u.Inserter.Put(ctx, rowSet)
				if err != nil {
					logging.WithError(err).Errorf(ctx, "eventupload: Uploader.Put failed")
					if merr, ok := err.(bigquery.PutMultiError); ok {
						if failed = len(merr); failed > len(rowSet) {
							logging.Errorf(ctx, "eventupload: %v failures trying to insert %v rows", failed, len(rowSet))
						}
					} else {
						failed = len(rowSet)
					}
					u.updateUploads(ctx, int64(failed), "failure")
				}
				succeeded := len(rowSet) - failed
				u.updateUploads(ctx, int64(succeeded), "success")
				return err
			}
		}
	})
}

func batch(rows []*Row, batchSize int) [][]*Row {
	rowSetsLen := int(math.Ceil(float64(len(rows) / batchSize)))
	rowSets := make([][]*Row, 0, rowSetsLen)
	for len(rows) > 0 {
		batch := rows
		if len(batch) > batchSize {
			batch = batch[:batchSize]
		}
		rowSets = append(rowSets, batch)
		rows = rows[len(batch):]
	}
	return rowSets
}
