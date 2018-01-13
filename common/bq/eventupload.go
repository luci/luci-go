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

// Package bq is a library for working with BigQuery.
package bq

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

// ID is the global InsertIDGenerator
var ID InsertIDGenerator

// reflect.FieldByName is O(N) in the number of fields. To avoid using it in
// mapFromMessage below, cache the field info for constant lookup.
type fieldInfo struct {
	structIndex       []int
	*proto.Properties // embedded
}

var bqFields = map[reflect.Type][]fieldInfo{}
var bqFieldsLock = sync.RWMutex{}

const insertLimit = 10000
const batchDefault = 500

// EventUploader is an interface for types which implement a Put method. It
// exists for the purpose of mocking Uploader in tests.
type EventUploader interface {
	Put(ctx context.Context, src interface{}) error
}

// Uploader contains the necessary data for streaming data to BigQuery.
type Uploader struct {
	*bigquery.Uploader
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

// Save is used by bigquery.Uploader.Put when inserting values into a table.
func (r *Row) Save() (map[string]bigquery.Value, string, error) {
	m, err := mapFromMessage(r.Message, nil)
	return m, r.InsertID, err
}

func mapFromMessage(m proto.Message, path []string) (map[string]bigquery.Value, error) {
	rowMap := map[string]bigquery.Value{}
	// GetProperties expects a Type with Kind Struct, not Ptr
	s := reflect.Indirect(reflect.ValueOf(m))
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid indirected value of %T", m)
	}

	t := s.Type()
	infos, err := getFieldInfos(t)
	if err != nil {
		return nil, errors.Annotate(err, "could not populate bqFields for type %v", t).Err()
	}
	path = append(path, "")
	for _, fi := range infos {
		var (
			value interface{}
			err   error
		)
		path[len(path)-1] = fi.Name
		if fi.Repeated {
			f := s.FieldByIndex(fi.structIndex)
			elems := make([]interface{}, f.Len())
			vPath := append(path, "")
			for i := 0; i < len(elems); i++ {
				vPath[len(vPath)-1] = strconv.Itoa(i)
				elems[i], err = getValue(f.Index(i).Interface(), vPath)
				if err != nil {
					return nil, err
				}
			}
			value = elems
		} else if fi.Enum != "" {
			stringer, ok := s.FieldByIndex(fi.structIndex).Interface().(fmt.Stringer)
			if !ok {
				return nil, fmt.Errorf("Could not convert enum value %s to string.", fi.Enum)
			}
			value = stringer.String()
		} else {
			value, err = getValue(s.FieldByIndex(fi.structIndex).Interface(), path)
			if err != nil {
				return nil, err
			}
		}
		rowMap[fi.OrigName] = bigquery.Value(value)
	}
	return rowMap, nil
}

func getFieldInfos(t reflect.Type) ([]fieldInfo, error) {
	bqFieldsLock.RLock()
	f := bqFields[t]
	bqFieldsLock.RUnlock()
	if f != nil {
		return f, nil
	}

	bqFieldsLock.Lock()
	defer bqFieldsLock.Unlock()

	if f := bqFields[t]; f != nil {
		return f, nil
	}

	props := proto.GetProperties(t).Prop
	fields := make([]fieldInfo, 0, len(props))
	for _, p := range props {
		f, ok := t.FieldByName(p.Name)
		switch {
		case !ok:
			return nil, fmt.Errorf("field %q not found in %q", p.Name, t)
		case p.OrigName == "":
			return nil, fmt.Errorf("OrigName of field %q.%q is empty", t, p.Name)
		}

		t := f.Type
		for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.PkgPath() == "github.com/golang/protobuf/ptypes/struct" {
			// Ignore protobuf structs, according to
			// https://godoc.org/go.chromium.org/luci/tools/cmd/bqschemaupdater
			continue
		}

		fields = append(fields, fieldInfo{f.Index, p})
	}

	bqFields[t] = fields
	return fields, nil
}

func getValue(value interface{}, path []string) (interface{}, error) {
	var err error
	if tspb, ok := value.(*timestamp.Timestamp); ok {
		value, err = ptypes.Timestamp(tspb)
		if err != nil {
			return nil, fmt.Errorf("tried to write an invalid timestamp for [%+v] for field %s", tspb, strings.Join(path, "."))
		}
	} else if nested, ok := value.(proto.Message); ok {
		value, err = mapFromMessage(nested, path)
		if err != nil {
			return nil, err
		}
	}
	return value, nil
}

// NewUploader constructs a new Uploader struct.
//
// DatasetID and TableID are provided to the BigQuery client to
// gain access to a particular table.
//
// You may want to change the default configuration of the bigquery.Uploader.
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
		Uploader:  c.Dataset(datasetID).Table(tableID).Uploader(),
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

func (u *Uploader) getCounter(ctx context.Context) metric.Counter {
	u.initMetricOnce.Do(func() {
		if u.UploadsMetricName != "" {
			desc := "Upload attempts; status is 'success' or 'failure'"
			field := field.String("status")
			u.uploads = metric.NewCounterIn(ctx, u.UploadsMetricName, desc, nil, field)
		}
	})
	return u.uploads
}

func (u *Uploader) updateUploads(ctx context.Context, count int64, status string) {
	uploads := u.getCounter(ctx)
	if uploads == nil || count == 0 {
		return
	}
	err := uploads.Add(ctx, count, status)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "eventupload: metric.Counter.Add failed")
	}
}

// Put uploads one or more rows to the BigQuery service. src is expected to
// be a struct (or a pointer to a struct) matching the schema in Uploader, or a
// slice containing such values. Put takes care of adding InsertIDs, used by
// BigQuery to deduplicate rows.
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
func (u *Uploader) Put(ctx context.Context, src interface{}) error {
	if _, ok := ctx.Deadline(); !ok {
		var c context.CancelFunc
		ctx, c = context.WithTimeout(ctx, time.Minute)
		defer c()
	}
	rows, err := rowsFromSrc(src)
	if err != nil {
		return err
	}
	for _, rowSet := range batch(rows, u.batchSize()) {
		var failed int
		err = u.Uploader.Put(ctx, rowSet)
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
		if err != nil {
			return err
		}
	}
	return nil
}

// TODO: when proto transition is complete, deal in *Rows, not bigquery.ValueSaver
func batch(rows []bigquery.ValueSaver, batchSize int) [][]bigquery.ValueSaver {
	rowSetsLen := int(math.Ceil(float64(len(rows) / batchSize)))
	rowSets := make([][]bigquery.ValueSaver, 0, rowSetsLen)
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

// rowsFromSrc accepts pointers to structs which implement proto.Message,
// and slices of this type. It does all necessary conversion and validation to
// return a slice of Row pointers. Row implements bigquery.ValueSaver and can be
// used with bigquery.Uploader.Put().
//
// rowsFromSrc also handles the following: structs which do not implement
// proto.Message, pointers to such structs, and slices of such values. This is
// to maintain backwards compatability during the proto transition.
// TODO: when proto transition is complete, deal in *Rows and proto.Messages.
func rowsFromSrc(src interface{}) ([]bigquery.ValueSaver, error) {
	// TODO: when proto transition is complete, remove
	validateSingleValue := func(v reflect.Value) error {
		switch v.Kind() {
		case reflect.Struct:
			return nil
		case reflect.Ptr:
			ptrK := v.Elem().Kind()
			if ptrK == reflect.Struct {
				return nil
			}
			return errors.Reason("pointer types must point to structs, not %s", ptrK).Err()
		default:
			return errors.Reason("struct or pointer-to-struct expected, got %v", v.Kind()).Err()
		}
	}

	vsFromVal := func(v reflect.Value) (bigquery.ValueSaver, error) {
		if m, ok := v.Interface().(proto.Message); ok {
			return &Row{
				Message:  m,
				InsertID: ID.Generate(),
			}, nil
		}
		// TODO: when proto transition is complete, remove
		err := validateSingleValue(v)
		if err != nil {
			return nil, err
		}
		s, err := bigquery.InferSchema(v)
		if err != nil {
			return nil, errors.Annotate(err, "could not infer schema for (%T)", v).Err()
		}
		return &bigquery.StructSaver{
			Schema:   s,
			InsertID: ID.Generate(),
			Struct:   v,
		}, nil
	}

	var vs []bigquery.ValueSaver
	srcV := reflect.ValueOf(src)
	if srcV.Kind() == reflect.Slice {
		vs = make([]bigquery.ValueSaver, srcV.Len())
		for i := 0; i < len(vs); i++ {
			v, err := vsFromVal(srcV.Index(i))
			if err != nil {
				return nil, err
			}
			vs[i] = v
		}
	} else {
		v, err := vsFromVal(srcV)
		if err != nil {
			return nil, err
		}
		vs = []bigquery.ValueSaver{v}
	}
	return vs, nil
}

// BatchUploader contains the necessary data for asynchronously sending batches
// of event row data to BigQuery.
type BatchUploader struct {
	u        EventUploader
	stopc    chan struct{}
	stoppedc chan struct{}

	mu      sync.Mutex
	pending []interface{}
	closed  int32
}

// NewBatchUploader constructs a new BatchUploader, which may optionally be
// further configured by setting its exported fields before the first call to
// Stage. Its Close method should be called when it is no longer needed.
//
// ctx is used by a goroutine, started when a BatchUploader is created by
// NewBatchUploader, that periodically uploads events. ctx should not be
// cancelled as a means of closing the BatchUploader, because it is needed for
// uploading buffered events. Instead, the client must call Close() on the
// BatchUploader. If ctx is cancelled before calling Close(), the goroutine will
// panic.
//
// Uploader implements EventUploader.
//
// c is a channel used by BatchUploader to prompt event upload. A
// <-chan time.Time with a ticker can be constructed with
// time.NewTicker(time.Duration).C. If left unset, the default upload
// interval is one minute.
func NewBatchUploader(ctx context.Context, u EventUploader, c <-chan time.Time) (*BatchUploader, error) {
	bu := &BatchUploader{
		u:        u,
		stopc:    make(chan struct{}),
		stoppedc: make(chan struct{}),
	}

	var ticker *time.Ticker
	if c == nil {
		ticker = time.NewTicker(time.Minute)
		c = ticker.C
	}

	go func() {
		if ticker != nil {
			defer ticker.Stop()
		}
		defer close(bu.stoppedc)
		for {
			select {
			case <-ctx.Done():
				panic("Context was closed before calling Close() on BatchUploader")
			case <-bu.stopc:
				// Final upload.
				bu.upload(ctx)
				return
			case <-c:
				bu.upload(ctx)
			}
		}
	}()

	return bu, nil
}

// Stage stages one or more rows for sending to BigQuery. src is expected to
// be a struct matching the schema in Uploader, or a slice containing
// such structs. Stage returns immediately and batches of rows will be sent to
// BigQuery at regular intervals.
//
// Stage will spawn another goroutine that manages uploads, if it hasn't been
// started already. That routine depends on ctx, so be aware that if ctx is
// cancelled immediately after calling Stage, those events will not be uploaded.
func (bu *BatchUploader) Stage(src interface{}) {
	if bu.isClosed() {
		panic("Stage called on closed BatchUploader")
	}

	bu.mu.Lock()
	defer bu.mu.Unlock()

	switch reflect.ValueOf(src).Kind() {
	case reflect.Slice:
		v := reflect.ValueOf(src)
		for i := 0; i < v.Len(); i++ {
			bu.pending = append(bu.pending, v.Index(i).Interface())
		}
	case reflect.Struct:
		bu.pending = append(bu.pending, src)
	}
}

// upload streams a batch of event rows to BigQuery. Put takes care of retrying,
// so if it returns an error there is either an issue with the data it is trying
// to upload, or BigQuery itself is experiencing a failure. So, we don't retry.
func (bu *BatchUploader) upload(ctx context.Context) {
	bu.mu.Lock()
	pending := bu.pending
	bu.pending = nil
	bu.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	_ = bu.u.Put(ctx, pending)
}

// Close flushes any pending event rows and releases any resources held by the
// uploader. Close should be called when the BatchUploader is no longer needed.
func (bu *BatchUploader) Close() {
	if atomic.CompareAndSwapInt32(&bu.closed, 0, 1) {
		close(bu.stopc)
		<-bu.stoppedc
	}
}

func (bu *BatchUploader) isClosed() bool {
	return atomic.LoadInt32(&bu.closed) == 1
}
