// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/prpc/internal/testpb"
)

func TestDecoding(t *testing.T) {
	t.Parallel()

	codecPicker := func(f Format) protoCodec {
		switch f {
		case FormatBinary:
			return codecWireV2
		case FormatJSONPB:
			return codecJSONV2
		case FormatText:
			return codecTextV2
		default:
			panic("impossible")
		}
	}

	ftt.Run("readMessage", t, func(t *ftt.Test) {
		const maxDecompressedSize = 100

		var msg testpb.HelloRequest
		read := func(contentType, contentEncoding string, body []byte) *protocolError {
			err := readMessage(
				bytes.NewBuffer(body),
				http.Header{
					"Content-Type":     {contentType},
					"Content-Encoding": {contentEncoding},
				},
				&msg,
				maxDecompressedSize,
				codecPicker,
			)
			if err != nil {
				return err.(*protocolError)
			}
			return nil
		}

		testLucy := func(t testing.TB, contentType string, body []byte) {
			t.Helper()
			err := read(contentType, "identity", body)
			assert.That(t, err, should.Equal[*protocolError](nil), truth.LineContext())
			assert.Loosely(t, &msg, should.Match(&testpb.HelloRequest{
				Name: "Lucy",
				Fields: &fieldmaskpb.FieldMask{
					Paths: []string{
						"name",
					},
				},
			}), truth.LineContext())
		}

		t.Run("binary", func(t *ftt.Test) {
			testMsg := &testpb.HelloRequest{
				Name: "Lucy",
				Fields: &fieldmaskpb.FieldMask{
					Paths: []string{
						"name",
					},
				},
			}
			body, err := proto.Marshal(testMsg)
			assert.Loosely(t, err, should.BeNil)

			t.Run(ContentTypePRPC, func(t *ftt.Test) {
				testLucy(t, ContentTypePRPC, body)
			})
			t.Run(mtPRPCBinary, func(t *ftt.Test) {
				testLucy(t, mtPRPCBinary, body)
			})
			t.Run("malformed body", func(t *ftt.Test) {
				err := read(mtPRPCBinary, "identity", []byte{0})
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.status, should.Equal(http.StatusBadRequest))
			})
			t.Run("empty body", func(t *ftt.Test) {
				err := read(mtPRPCBinary, "identity", nil)
				assert.Loosely(t, err, should.Equal[*protocolError](nil))
			})
		})

		t.Run("json", func(t *ftt.Test) {
			body := []byte(`{"name": "Lucy", "fields": "name"}`)
			t.Run(ContentTypeJSON, func(t *ftt.Test) {
				testLucy(t, ContentTypeJSON, body)
			})
			t.Run(mtPRPCJSONPBLegacy, func(t *ftt.Test) {
				testLucy(t, mtPRPCJSONPB, body)
			})
			t.Run("malformed body", func(t *ftt.Test) {
				err := read(mtPRPCJSONPB, "identity", []byte{0})
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.status, should.Equal(http.StatusBadRequest))
			})
			t.Run("empty body", func(t *ftt.Test) {
				err := read(mtPRPCJSONPB, "identity", nil)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.status, should.Equal(http.StatusBadRequest))
			})
		})

		t.Run("text", func(t *ftt.Test) {
			t.Run(mtPRPCText, func(t *ftt.Test) {
				body := []byte(`name: "Lucy" fields < paths: "name" >`)
				testLucy(t, mtPRPCText, body)
			})
			t.Run("malformed body", func(t *ftt.Test) {
				err := read(mtPRPCText, "identity", []byte{0})
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.status, should.Equal(http.StatusBadRequest))
			})
			t.Run("empty body", func(t *ftt.Test) {
				err := read(mtPRPCText, "identity", nil)
				assert.Loosely(t, err, should.Equal[*protocolError](nil))
			})
		})

		t.Run("unsupported media type", func(t *ftt.Test) {
			err := read("blah", "identity", nil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.status, should.Equal(http.StatusUnsupportedMediaType))
		})

		t.Run("compressed", func(t *ftt.Test) {
			compressRaw := func(blob []byte) []byte {
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				_, err := w.Write(blob)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, w.Close(), should.BeNil)
				return buf.Bytes()
			}

			t.Run("OK", func(t *ftt.Test) {
				m := &testpb.HelloRequest{Name: "hi"}
				blob, err := proto.Marshal(m)
				assert.Loosely(t, err, should.BeNil)

				err = read(mtPRPCBinary, "gzip", compressRaw(blob))
				assert.Loosely(t, err, should.Equal[*protocolError](nil))
				assert.Loosely(t, msg.Name, should.Equal("hi"))
			})

			t.Run("Exactly at the limit", func(t *ftt.Test) {
				err := read(mtPRPCBinary, "gzip",
					compressRaw(make([]byte, maxDecompressedSize)))
				assert.Loosely(t, err, should.ErrLike("could not decode body")) // it is full of zeros
			})

			t.Run("Past the limit", func(t *ftt.Test) {
				err := read(mtPRPCBinary, "gzip",
					compressRaw(make([]byte, maxDecompressedSize+1)))
				assert.Loosely(t, err, should.ErrLike("the decompressed request size exceeds the server limit"))
			})
		})
	})

	ftt.Run("parseHeader", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("host", func(t *ftt.Test) {
			c, _, err := parseHeader(c, http.Header{}, "example.com")
			assert.Loosely(t, err, should.BeNil)
			md, ok := metadata.FromIncomingContext(c)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, md.Get("host"), should.Match([]string{"example.com"}))
		})

		header := func(name, value string) http.Header {
			return http.Header{name: []string{value}}
		}
		parse := func(c context.Context, name, value string) (context.Context, error) {
			ctx, _, err := parseHeader(c, header(name, value), "")
			return ctx, err
		}

		t.Run(HeaderTimeout, func(t *ftt.Test) {
			t.Run("Works", func(t *ftt.Test) {
				now := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)
				c, _ = testclock.UseTime(c, now)

				var err error
				c, err = parse(c, HeaderTimeout, "1M")
				assert.Loosely(t, err, should.BeNil)

				deadline, ok := c.Deadline()
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, deadline, should.HappenWithin(time.Second, now.Add(time.Minute)))
			})

			t.Run("Fails", func(t *ftt.Test) {
				c, err := parse(c, HeaderTimeout, "blah")
				assert.Loosely(t, c, should.Equal(c))
				assert.Loosely(t, err, should.ErrLike(`"`+HeaderTimeout+`" header: unit is not recognized: "blah"`))
			})
		})

		t.Run("Content-Type", func(t *ftt.Test) {
			c, err := parse(c, "Content-Type", "blah")
			assert.Loosely(t, err, should.BeNil)
			_, ok := metadata.FromIncomingContext(c)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Accept", func(t *ftt.Test) {
			c, err := parse(c, "Accept", "blah")
			assert.Loosely(t, err, should.BeNil)
			_, ok := metadata.FromIncomingContext(c)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Unrecognized headers", func(t *ftt.Test) {
			test := func(ctx context.Context, header http.Header, expectedMetadata metadata.MD) {
				ctx, _, err := parseHeader(ctx, header, "")
				assert.Loosely(t, err, should.BeNil)
				md, ok := metadata.FromIncomingContext(ctx)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, md, should.Match(expectedMetadata))
			}

			headers := http.Header{
				"X": []string{"1"},
				"Y": []string{"1", "2"},
			}

			t.Run("without metadata in context", func(t *ftt.Test) {
				test(c, headers, metadata.MD{
					"x": []string{"1"},
					"y": []string{"1", "2"},
				})
			})

			t.Run("with metadata in context", func(t *ftt.Test) {
				c = metadata.NewIncomingContext(c, metadata.MD{
					"x": []string{"0"},
					"z": []string{"1"},
				})
				test(c, headers, metadata.MD{
					"x": []string{"0", "1"},
					"y": []string{"1", "2"},
					"z": []string{"1"},
				})
			})

			t.Run("binary", func(t *ftt.Test) {
				t.Run("Works", func(t *ftt.Test) {
					const name = "Lucy"
					b64 := base64.StdEncoding.EncodeToString([]byte(name))
					test(c, header("Name-Bin", b64), metadata.MD{
						"name-bin": []string{name},
					})
				})
				t.Run("Fails", func(t *ftt.Test) {
					c, err := parse(c, "Name-Bin", "zzz")
					assert.Loosely(t, c, should.Equal(c))
					assert.Loosely(t, err, should.ErrLike(`header "Name-Bin": illegal base64 data at input byte 0`))
				})
			})
		})
	})
}
