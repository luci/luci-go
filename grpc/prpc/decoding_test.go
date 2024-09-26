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

	"github.com/golang/protobuf/proto"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDecoding(t *testing.T) {
	t.Parallel()

	Convey("readMessage", t, func() {
		const maxDecompressedSize = 100

		var msg HelloRequest
		read := func(contentType, contentEncoding string, body []byte) *protocolError {
			err := readMessage(
				bytes.NewBuffer(body),
				http.Header{
					"Content-Type":     {contentType},
					"Content-Encoding": {contentEncoding},
				},
				&msg,
				maxDecompressedSize,
				true,
			)
			if err != nil {
				return err.(*protocolError)
			}
			return nil
		}

		testLucy := func(contentType string, body []byte) {
			err := read(contentType, "identity", body)
			So(err, ShouldBeNil)
			So(&msg, ShouldResembleProto, &HelloRequest{
				Name: "Lucy",
				Fields: &field_mask.FieldMask{
					Paths: []string{
						"name",
					},
				},
			})
		}

		Convey("binary", func() {
			testMsg := &HelloRequest{
				Name: "Lucy",
				Fields: &field_mask.FieldMask{
					Paths: []string{
						"name",
					},
				},
			}
			body, err := proto.Marshal(testMsg)
			So(err, ShouldBeNil)

			Convey(ContentTypePRPC, func() {
				testLucy(ContentTypePRPC, body)
			})
			Convey(mtPRPCBinary, func() {
				testLucy(mtPRPCBinary, body)
			})
			Convey("malformed body", func() {
				err := read(mtPRPCBinary, "identity", []byte{0})
				So(err, ShouldNotBeNil)
				So(err.status, ShouldEqual, http.StatusBadRequest)
			})
			Convey("empty body", func() {
				err := read(mtPRPCBinary, "identity", nil)
				So(err, ShouldBeNil)
			})
		})

		Convey("json", func() {
			body := []byte(`{"name": "Lucy", "fields": "name"}`)
			Convey(ContentTypeJSON, func() {
				testLucy(ContentTypeJSON, body)
			})
			Convey(mtPRPCJSONPBLegacy, func() {
				testLucy(mtPRPCJSONPB, body)
			})
			Convey("malformed body", func() {
				err := read(mtPRPCJSONPB, "identity", []byte{0})
				So(err, ShouldNotBeNil)
				So(err.status, ShouldEqual, http.StatusBadRequest)
			})
			Convey("empty body", func() {
				err := read(mtPRPCJSONPB, "identity", nil)
				So(err, ShouldNotBeNil)
				So(err.status, ShouldEqual, http.StatusBadRequest)
			})
		})

		Convey("text", func() {
			Convey(mtPRPCText, func() {
				body := []byte(`name: "Lucy" fields < paths: "name" >`)
				testLucy(mtPRPCText, body)
			})
			Convey("malformed body", func() {
				err := read(mtPRPCText, "identity", []byte{0})
				So(err, ShouldNotBeNil)
				So(err.status, ShouldEqual, http.StatusBadRequest)
			})
			Convey("empty body", func() {
				err := read(mtPRPCText, "identity", nil)
				So(err, ShouldBeNil)
			})
		})

		Convey("unsupported media type", func() {
			err := read("blah", "identity", nil)
			So(err, ShouldNotBeNil)
			So(err.status, ShouldEqual, http.StatusUnsupportedMediaType)
		})

		Convey("compressed", func() {
			compressRaw := func(blob []byte) []byte {
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				_, err := w.Write(blob)
				So(err, ShouldBeNil)
				So(w.Close(), ShouldBeNil)
				return buf.Bytes()
			}

			Convey("OK", func() {
				m := &HelloRequest{Name: "hi"}
				blob, err := proto.Marshal(m)
				So(err, ShouldBeNil)

				err = read(mtPRPCBinary, "gzip", compressRaw(blob))
				So(err, ShouldBeNil)
				So(msg.Name, ShouldEqual, "hi")
			})

			Convey("Exactly at the limit", func() {
				err := read(mtPRPCBinary, "gzip",
					compressRaw(make([]byte, maxDecompressedSize)))
				So(err, ShouldErrLike, "could not decode body") // it is full of zeros
			})

			Convey("Past the limit", func() {
				err := read(mtPRPCBinary, "gzip",
					compressRaw(make([]byte, maxDecompressedSize+1)))
				So(err, ShouldErrLike, "the decompressed request size exceeds the server limit")
			})
		})
	})

	Convey("parseHeader", t, func() {
		c := context.Background()

		Convey("host", func() {
			c, _, err := parseHeader(c, http.Header{}, "example.com")
			So(err, ShouldBeNil)
			md, ok := metadata.FromIncomingContext(c)
			So(ok, ShouldBeTrue)
			So(md.Get("host"), ShouldResemble, []string{"example.com"})
		})

		header := func(name, value string) http.Header {
			return http.Header{name: []string{value}}
		}
		parse := func(c context.Context, name, value string) (context.Context, error) {
			ctx, _, err := parseHeader(c, header(name, value), "")
			return ctx, err
		}

		Convey(HeaderTimeout, func() {
			Convey("Works", func() {
				now := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)
				c, _ = testclock.UseTime(c, now)

				var err error
				c, err = parse(c, HeaderTimeout, "1M")
				So(err, ShouldBeNil)

				deadline, ok := c.Deadline()
				So(ok, ShouldBeTrue)
				So(deadline, ShouldHappenWithin, time.Second, now.Add(time.Minute))
			})

			Convey("Fails", func() {
				c, err := parse(c, HeaderTimeout, "blah")
				So(c, ShouldEqual, c)
				So(err, ShouldErrLike, `"`+HeaderTimeout+`" header: unit is not recognized: "blah"`)
			})
		})

		Convey("Content-Type", func() {
			c, err := parse(c, "Content-Type", "blah")
			So(err, ShouldBeNil)
			_, ok := metadata.FromIncomingContext(c)
			So(ok, ShouldBeFalse)
		})

		Convey("Accept", func() {
			c, err := parse(c, "Accept", "blah")
			So(err, ShouldBeNil)
			_, ok := metadata.FromIncomingContext(c)
			So(ok, ShouldBeFalse)
		})

		Convey("Unrecognized headers", func() {
			test := func(ctx context.Context, header http.Header, expectedMetadata metadata.MD) {
				ctx, _, err := parseHeader(ctx, header, "")
				So(err, ShouldBeNil)
				md, ok := metadata.FromIncomingContext(ctx)
				So(ok, ShouldBeTrue)
				So(md, ShouldResemble, expectedMetadata)
			}

			headers := http.Header{
				"X": []string{"1"},
				"Y": []string{"1", "2"},
			}

			Convey("without metadata in context", func() {
				test(c, headers, metadata.MD{
					"x": []string{"1"},
					"y": []string{"1", "2"},
				})
			})

			Convey("with metadata in context", func() {
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

			Convey("binary", func() {
				Convey("Works", func() {
					const name = "Lucy"
					b64 := base64.StdEncoding.EncodeToString([]byte(name))
					test(c, header("Name-Bin", b64), metadata.MD{
						"name-bin": []string{name},
					})
				})
				Convey("Fails", func() {
					c, err := parse(c, "Name-Bin", "zzz")
					So(c, ShouldEqual, c)
					So(err, ShouldErrLike, `header "Name-Bin": illegal base64 data at input byte 0`)
				})
			})
		})
	})
}
