// Copyright 2015 The LUCI Authors.
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

package pubsubprotocol

import (
	"bytes"
	"io"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
)

func read(ir io.Reader) (*Reader, error) {
	r := Reader{}
	if err := r.Read(ir); err != nil {
		return nil, err
	}
	return &r, nil
}

func TestReader(t *testing.T) {
	ftt.Run(`A Reader instance`, t, func(t *ftt.Test) {
		r := Reader{}
		buf := bytes.Buffer{}
		fw := recordio.NewWriter(&buf)

		writeFrame := func(data []byte) error {
			if _, err := fw.Write(data); err != nil {
				return err
			}
			if err := fw.Flush(); err != nil {
				return err
			}
			return nil
		}

		push := func(m proto.Message) {
			data, err := proto.Marshal(m)
			if err != nil {
				panic(err)
			}
			if err := writeFrame(data); err != nil {
				panic(err)
			}
		}

		// Test case templates.
		md := logpb.ButlerMetadata{
			Type:         logpb.ButlerMetadata_ButlerLogBundle,
			ProtoVersion: logpb.Version,
		}
		bundle := logpb.ButlerLogBundle{}

		t.Run(`Can read a ButlerLogBundle entry.`, func(t *ftt.Test) {
			push(&md)
			push(&bundle)

			assert.Loosely(t, r.Read(&buf), should.BeNil)
			assert.Loosely(t, r.Bundle, should.NotBeNil)
		})

		t.Run(`Will fail to Read an unknown type.`, func(t *ftt.Test) {
			// Assert that we are testing an unknown type.
			unknownType := logpb.ButlerMetadata_ContentType(-1)
			assert.Loosely(t, logpb.ButlerMetadata_ContentType_name[int32(unknownType)], should.BeEmpty)

			md.Type = unknownType
			push(&md)

			err := r.Read(&buf)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike("unknown data type"))
		})

		t.Run(`Will not decode contents if an unknown protocol version is identified.`, func(t *ftt.Test) {
			// Assert that we are testing an unknown type.
			md.ProtoVersion = "DEFINITELY NOT VALID"
			push(&md)
			push(&bundle)

			assert.Loosely(t, r.Read(&buf), should.BeNil)
			assert.Loosely(t, r.Bundle, should.BeNil)
		})

		t.Run(`Will fail to read junk metadata.`, func(t *ftt.Test) {
			assert.Loosely(t, writeFrame([]byte{0xd0, 0x6f, 0x00, 0xd5}), should.BeNil)

			err := r.Read(&buf)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal Metadata frame"))
		})

		t.Run(`With a proper Metadata frame`, func(t *ftt.Test) {
			push(&md)

			t.Run(`Will fail if the bundle data is junk.`, func(t *ftt.Test) {
				assert.Loosely(t, writeFrame([]byte{0xd0, 0x6f, 0x00, 0xd5}), should.BeNil)

				err := r.Read(&buf)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("failed to unmarshal Bundle frame"))
			})
		})

		t.Run(`With a proper compressed Metadata frame`, func(t *ftt.Test) {
			md.Compression = logpb.ButlerMetadata_ZLIB
			push(&md)

			t.Run(`Will fail if the data frame is missing.`, func(t *ftt.Test) {
				err := r.Read(&buf)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("failed to read Bundle data"))
			})

			t.Run(`Will fail if there is junk compressed data.`, func(t *ftt.Test) {
				assert.Loosely(t, writeFrame(bytes.Repeat([]byte{0x55, 0xAA}, 16)), should.BeNil)

				err := r.Read(&buf)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("failed to initialize zlib reader"))
			})
		})

		t.Run(`Will refuse to read a frame larger than our maximum size.`, func(t *ftt.Test) {
			r.maxSize = 16
			assert.Loosely(t, writeFrame(bytes.Repeat([]byte{0x55}, 17)), should.BeNil)

			err := r.Read(&buf)
			assert.Loosely(t, err, should.Equal(recordio.ErrFrameTooLarge))
		})

		t.Run(`Will refuse to read a compressed protobuf larger than our maximum size.`, func(t *ftt.Test) {
			// We are crafting this data such that its compressed (frame) size is
			// below our threshold (16), but its compressed size exceeds it. Repeated
			// bytes compress very well :)
			//
			// Because the frame it smaller than our threshold, our FrameReader will
			// not outright reject the frame. However, the data is still larger than
			// we're allowed, and we must reject it.
			r.maxSize = 16
			w := Writer{
				Compress:          true,
				CompressThreshold: 0,
			}
			assert.Loosely(t, w.writeData(recordio.NewWriter(&buf), logpb.ButlerMetadata_ButlerLogBundle,
				bytes.Repeat([]byte{0x55}, 64)), should.BeNil)

			err := r.Read(&buf)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err, should.ErrLike("limit exceeded"))
		})
	})
}

func TestWriter(t *testing.T) {
	ftt.Run(`A Writer instance outputting to a Buffer`, t, func(t *ftt.Test) {
		buf := bytes.Buffer{}
		w := Writer{}
		bundle := logpb.ButlerLogBundle{
			Entries: []*logpb.ButlerLogBundle_Entry{
				{},
			},
		}

		t.Run(`When configured to compress with a threshold of 64`, func(t *ftt.Test) {
			w.Compress = true
			w.CompressThreshold = 64

			t.Run(`Will not compress if below the compression threshold.`, func(t *ftt.Test) {
				assert.Loosely(t, w.Write(&buf, &bundle), should.BeNil)

				r, err := read(&buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, r.Metadata.Compression, should.Equal(logpb.ButlerMetadata_NONE))
				assert.Loosely(t, r.Metadata.ProtoVersion, should.Equal(logpb.Version))
			})

			t.Run(`Will not write data larger than the maximum bundle size.`, func(t *ftt.Test) {
				w.maxSize = 16
				bundle.Secret = bytes.Repeat([]byte{'A'}, 17)
				err := w.Write(&buf, &bundle)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("exceeds soft cap"))
			})

			t.Run(`Will compress data >= the threshold.`, func(t *ftt.Test) {
				bundle.Secret = bytes.Repeat([]byte{'A'}, 64)
				assert.Loosely(t, w.Write(&buf, &bundle), should.BeNil)

				r, err := read(&buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, r.Metadata.Compression, should.Equal(logpb.ButlerMetadata_ZLIB))
				assert.Loosely(t, r.Metadata.ProtoVersion, should.Equal(logpb.Version))

				t.Run(`And can be reused.`, func(t *ftt.Test) {
					assert.Loosely(t, w.Write(&buf, &bundle), should.BeNil)

					r, err := read(&buf)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, r.Metadata.Compression, should.Equal(logpb.ButlerMetadata_ZLIB))
					assert.Loosely(t, r.Metadata.ProtoVersion, should.Equal(logpb.Version))
				})
			})
		})
	})
}
