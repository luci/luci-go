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

package model

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/compression"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	// BuildStepsKind is a BuildSteps entity's kind in the datastore.
	BuildStepsKind = "BuildSteps"

	// BuildInfraKind is a BuildInfra entity's kind in the datastore.
	BuildInfraKind = "BuildInfra"

	// BuildInputPropertiesKind is a BuildInputProperties entity's kind in the datastore.
	BuildInputPropertiesKind = "BuildInputProperties"

	// BuildOutputPropertiesKind is a BuildOutputProperties entity's kind in the datastore.
	BuildOutputPropertiesKind = "BuildOutputProperties"
)

// maxPropertySize is the maximum property size. Any properties larger than it
// should be chunked into multiple entities to fit the Datastore size limit.
// This value is smaller than the real Datastore limit (1048487 bytes) in order
// to give some headroom.
// Note: make it to var instead of const to favor our unit tests. Otherwise, it
// will take up too much memory when covering different test cases (e.g,
// compressed property bytes > 4 chunks)
var maxPropertySize = 1000 * 1000

// defaultStructValues defaults nil or empty values inside the given
// structpb.Struct. Needed because structpb.Value cannot be marshaled to JSON
// unless there is a kind set.
func defaultStructValues(s *structpb.Struct) {
	for k, v := range s.GetFields() {
		switch {
		case v == nil:
			s.Fields[k] = &structpb.Value{
				Kind: &structpb.Value_NullValue{},
			}
		case v.Kind == nil:
			v.Kind = &structpb.Value_NullValue{}
		case v.GetStructValue() != nil:
			defaultStructValues(v.GetStructValue())
		}
	}
}

// BuildInfra is a representation of a build proto's infra field
// in the datastore.
type BuildInfra struct {
	_kind string `gae:"$kind,BuildInfra"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the pb.BuildInfra proto representation of the infra field.
	Proto *pb.BuildInfra `gae:"infra,legacy"`
}

var _ datastore.PropertyLoadSaver = (*BuildInfra)(nil)

// Load implements datastore.PropertyLoadSaver in order to apply
// defaultStructValues to bi.Proto.
func (bi *BuildInfra) Load(pm datastore.PropertyMap) error {
	if err := datastore.GetPLS(bi).Load(pm); err != nil {
		return err
	}
	if bi.Proto.GetBuildbucket() != nil {
		defaultStructValues(bi.Proto.Buildbucket.RequestedProperties)
	}
	return nil
}

// Save implements datastore.PropertyLoadSaver
func (bi *BuildInfra) Save(withMeta bool) (datastore.PropertyMap, error) {
	return datastore.GetPLS(bi).Save(withMeta)
}

// BuildInputProperties is a representation of a build proto's input field's
// properties field in the datastore.
type BuildInputProperties struct {
	_kind string `gae:"$kind,BuildInputProperties"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the structpb.Struct representation of the properties field.
	Proto *structpb.Struct `gae:"properties,legacy"`
}

// BuildOutputProperties is a representation of a build proto's output field's
// properties field in the datastore.
//
// Note: avoid directly access to BuildOutputProperties via datastore.Get and
// datastore.Put, as it may be chunked if it exceeds maxPropertySize.
// Please always use *BuildOutputProperties.Get and *BuildOutputProperties.Put.
type BuildOutputProperties struct {
	_     datastore.PropertyMap `gae:"-,extra"`
	_kind string                `gae:"$kind,BuildOutputProperties"`
	// _id is always 1 because only one such entity exists.
	_id int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the structpb.Struct representation of the properties field.
	Proto *structpb.Struct `gae:"properties,legacy"`

	// ChunkCount indicates how many chunks this Proto is splitted into.
	ChunkCount int `gae:"chunk_count,noindex"`
}

// PropertyChunk stores a chunk of serialized and compressed
// BuildOutputProperties.Proto bytes.
// In the future, it may expand to buildInputProperties.
type PropertyChunk struct {
	_kind string `gae:"$kind,PropertyChunk"`
	// ID starts from 1 to N where N is BuildOutputProperties.ChunkCount.
	ID int `gae:"$id"`
	// The BuildOutputProperties entity that this entity belongs to.
	Parent *datastore.Key `gae:"$parent"`

	// chunked bytes
	Bytes []byte `gae:"chunk,noindex"`
}

// chunkProp splits BuildOutputProperties into chunks and return them. The nil
// return means it doesn't need to chunk.
// Note: The caller is responsible for putting the chunks into Datastore.
func (bo *BuildOutputProperties) chunkProp(c context.Context) ([]*PropertyChunk, error) {
	if bo == nil || bo.Proto == nil || bo.Build == nil {
		return nil, nil
	}
	propBytes, err := proto.Marshal(bo.Proto)
	if err != nil {
		return nil, errors.Annotate(err, "failed to marshal build output properties").Err()
	}
	if len(propBytes) <= maxPropertySize {
		return nil, nil
	}

	// compress propBytes
	compressed := make([]byte, 0, len(propBytes)/2) // hope for at least 2x compression
	compressed = compression.ZstdCompress(propBytes, compressed)

	// to round up the result of integer division.
	count := (len(compressed) + maxPropertySize - 1) / maxPropertySize
	chunks := make([]*PropertyChunk, count)
	pk := datastore.KeyForObj(c, &BuildOutputProperties{
		Build: datastore.KeyForObj(c, &Build{ID: bo.Build.IntID()}),
	})
	for i := 0; i < count; i++ {
		idxStart := i * maxPropertySize
		idxEnd := idxStart + maxPropertySize
		if idxEnd > len(compressed) {
			idxEnd = len(compressed)
		}

		chunks[i] = &PropertyChunk{
			ID:     i + 1, // ID starts from 1.
			Parent: pk,
			Bytes:  compressed[idxStart:idxEnd],
		}
	}
	return chunks, nil
}

// Get is a wrapper of `datastore.Get` to properly handle large properties.
func (bo *BuildOutputProperties) Get(c context.Context) error {
	if bo == nil || bo.Build == nil {
		return nil
	}

	pk := datastore.KeyForObj(c, &BuildOutputProperties{
		Build: datastore.KeyForObj(c, &Build{ID: bo.Build.IntID()}),
	})
	// Preemptively fetch up to 4 chunks to minimize Datastore RPC calls so that
	// in most cases, it only needs one call.
	//
	// BUG(b/258241457) - Setting this to 0 to see if it tamps down suprious
	// Lookup costs in datastore. Should evaluate re-enabling after we turn
	// entity caching back on.
	const preFetchedChunkCnt = 0
	chunks := make([]*PropertyChunk, preFetchedChunkCnt)
	for i := 0; i < preFetchedChunkCnt; i++ {
		chunks[i] = &PropertyChunk{
			ID:     i + 1, // ID starts from 1.
			Parent: pk,
		}
	}

	if err := datastore.Get(c, bo, chunks); err != nil {
		switch me, ok := err.(errors.MultiError); {
		case !ok:
			return err
		case me[0] != nil:
			return me[0]
		case errors.Filter(me[1], datastore.ErrNoSuchEntity) != nil:
			return errors.Fmt("fail to fetch first %d chunks for BuildOutputProperties: %w", preFetchedChunkCnt, me[1])
		}
	}

	// No chunks.
	if bo.ChunkCount == 0 {
		return nil
	}

	// Fetch the rest chunks.
	if bo.ChunkCount-preFetchedChunkCnt > 0 {
		for i := preFetchedChunkCnt + 1; i <= bo.ChunkCount; i++ {
			chunks = append(chunks, &PropertyChunk{
				ID:     i,
				Parent: pk,
			})
		}

		if err := datastore.Get(c, chunks[preFetchedChunkCnt:]); err != nil {
			return errors.Annotate(err, "failed to fetch the rest chunks for BuildOutputProperties").Err()
		}
	}
	chunks = chunks[:bo.ChunkCount]

	// Assemble proto bytes and restore to proto.
	var compressedBytes []byte
	for _, chunk := range chunks {
		compressedBytes = append(compressedBytes, chunk.Bytes...)
	}
	var propBytes []byte
	var err error
	if propBytes, err = compression.ZstdDecompress(compressedBytes, nil); err != nil {
		return errors.Annotate(err, "failed to decompress output properties bytes").Err()
	}
	bo.Proto = &structpb.Struct{}
	if err := proto.Unmarshal(propBytes, bo.Proto); err != nil {
		return errors.Annotate(err, "failed to unmarshal outputProperties' chunks").Err()
	}
	bo.ChunkCount = 0
	return nil
}

// GetMultiOutputProperties fetches multiple BuildOutputProperties in parallel.
func GetMultiOutputProperties(c context.Context, props ...*BuildOutputProperties) error {
	nWorkers := 8
	if len(props) < nWorkers {
		nWorkers = len(props)
	}

	err := parallel.WorkPool(nWorkers, func(work chan<- func() error) {
		for _, prop := range props {
			if prop == nil || prop.Build == nil {
				continue
			}
			work <- func() error {
				return prop.Get(c)
			}
		}
	})
	return err
}

// Put is a wrapper of `datastore.Put` to properly handle large properties.
// Suggest calling it in a transaction to correctly handle partial failures when
// putting PropertyChunk and BuildOutputProperties.
func (bo *BuildOutputProperties) Put(c context.Context) error {
	if bo == nil || bo.Build == nil {
		return nil
	}

	chunks, err := bo.chunkProp(c)
	if err != nil {
		return err
	}

	prop := bo.Proto
	if len(chunks) != 0 {
		bo.Proto = nil
		bo.ChunkCount = len(chunks)
	} else {
		bo.ChunkCount = 0
	}

	if err := datastore.Put(c, bo, chunks); err != nil {
		return err
	}
	bo.Proto = prop
	return nil
}

// BuildStepsMaxBytes is the maximum length of BuildSteps.Bytes. If Bytes
// exceeds this maximum, this package will try to compress it, setting IsZipped
// accordingly, but if this length is still exceeded it's an error to write
// such entities to the datastore. Use FromProto to ensure this maximum is
// respected.
const BuildStepsMaxBytes = 1e6

// BuildSteps is a representation of a build proto's steps field
// in the datastore.
type BuildSteps struct {
	_kind string `gae:"$kind,BuildSteps"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// IsZipped indicates whether or not Bytes is zlib compressed.
	// Use ToProto to ensure this compression is respected.
	IsZipped bool `gae:"step_container_bytes_zipped,noindex"`
	// Bytes is the pb.Build proto representation of the build proto where only steps is set.
	// IsZipped determines whether this value is compressed or not.
	Bytes []byte `gae:"steps,noindex"`
}

// CancelIncomplete marks any incomplete steps as cancelled, returning whether
// at least one step was cancelled. The caller is responsible for writing the
// entity to the datastore if any steps were cancelled. This entity will not be
// mutated if an error occurs.
func (s *BuildSteps) CancelIncomplete(ctx context.Context, now *timestamppb.Timestamp) (bool, error) {
	stp, err := s.ToProto(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for _, s := range stp {
		if !protoutil.IsEnded(s.Status) {
			s.EndTime = now
			s.Status = pb.Status_CANCELED
			changed = true
		}
	}
	if changed {
		if err := s.FromProto(stp); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// FromProto overwrites the current []*pb.Step representation of these steps.
// The caller is responsible for writing the entity to the datastore. This
// entity will not be mutated if an error occurs.
func (s *BuildSteps) FromProto(stp []*pb.Step) error {
	b, err := proto.Marshal(&pb.Build{
		Steps: stp,
	})
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	if len(b) <= BuildStepsMaxBytes {
		s.Bytes = b
		s.IsZipped = false
		return nil
	}
	buf := &bytes.Buffer{}
	w := zlib.NewWriter(buf)
	if _, err := w.Write(b); err != nil {
		return errors.Annotate(err, "error zipping").Err()
	}
	if err := w.Close(); err != nil {
		return errors.Annotate(err, "error closing writer").Err()
	}
	s.Bytes = buf.Bytes()
	s.IsZipped = true
	return nil
}

// ToProto returns the []*pb.Step representation of these steps.
func (s *BuildSteps) ToProto(ctx context.Context) ([]*pb.Step, error) {
	b := s.Bytes
	if s.IsZipped {
		r, err := zlib.NewReader(bytes.NewReader(s.Bytes))
		if err != nil {
			return nil, errors.Annotate(err, "error creating reader for %q", datastore.KeyForObj(ctx, s)).Err()
		}
		b, err = io.ReadAll(r)
		if err != nil {
			return nil, errors.Annotate(err, "error reading %q", datastore.KeyForObj(ctx, s)).Err()
		}
		if err := r.Close(); err != nil {
			return nil, errors.Annotate(err, "error closing reader for %q", datastore.KeyForObj(ctx, s)).Err()
		}
	}
	p := &pb.Build{}
	if err := proto.Unmarshal(b, p); err != nil {
		return nil, errors.Annotate(err, "error unmarshalling %q", datastore.KeyForObj(ctx, s)).Err()
	}
	return p.Steps, nil
}
