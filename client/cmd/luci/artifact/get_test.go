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

package artifact

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestParseByteRange(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseByteRange`, t, func(t *ftt.Test) {
		t.Run(`empty`, func(t *ftt.Test) {
			br, err := ParseByteRange("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.BeNil)
		})

		t.Run(`start and end`, func(t *ftt.Test) {
			br, err := ParseByteRange("0-100")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.Resemble(&ByteRange{Start: 0, End: 100}))

			br2, err := ParseByteRange("10:50")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br2, should.Resemble(&ByteRange{Start: 10, End: 50}))
		})

		t.Run(`start to EOF`, func(t *ftt.Test) {
			br, err := ParseByteRange("500-")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.Resemble(&ByteRange{Start: 500, End: -1}))

			br2, err := ParseByteRange("250")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br2, should.Resemble(&ByteRange{Start: 250, End: -1}))
		})

		t.Run(`suffix range`, func(t *ftt.Test) {
			br, err := ParseByteRange("-500")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, br, should.Resemble(&ByteRange{Start: -1, End: 500}))
		})

		t.Run(`invalid ranges`, func(t *ftt.Test) {
			_, err := ParseByteRange("abc")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseByteRange("100-50")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = ParseByteRange("-0")
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestFetchHTTPByteRange(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHTTPByteRange`, t, func(t *ftt.Test) {
		ctx := context.Background()
		fullContent := "line 1: hello\nline 2: world\nline 3: luci\nline 4: resultdb\n"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rangeHdr := r.Header.Get("Range")
			if rangeHdr == "bytes=0-14" {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[:15]))
				return
			}
			if rangeHdr == "bytes=15-" {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[15:]))
				return
			}
			if rangeHdr == "bytes=-10" {
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[len(fullContent)-10:]))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fullContent))
		}))
		defer server.Close()

		t.Run(`full content`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, nil, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent))
		})

		t.Run(`server partial content range start-end`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, &ByteRange{Start: 0, End: 14}, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent[:15]))
		})

		t.Run(`server partial content range start to EOF`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, &ByteRange{Start: 15, End: -1}, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent[15:]))
		})

		t.Run(`server partial content suffix range`, func(t *ftt.Test) {
			var buf bytes.Buffer
			_, _, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, &ByteRange{Start: -1, End: 10}, &buf)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf.String(), should.Equal(fullContent[len(fullContent)-10:]))
		})

		t.Run(`server ignores Range and returns 200 OK`, func(t *ftt.Test) {
			okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fullContent))
			}))
			defer okServer.Close()

			t.Run(`range start-end from 0`, func(t *ftt.Test) {
				var buf bytes.Buffer
				status, _, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: 0, End: 14}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Equal(http.StatusOK))
				assert.Loosely(t, buf.String(), should.Equal(fullContent[:15]))
			})

			t.Run(`range start-end from middle`, func(t *ftt.Test) {
				var buf bytes.Buffer
				status, _, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: 14, End: 27}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Equal(http.StatusOK))
				assert.Loosely(t, buf.String(), should.Equal(fullContent[14:28]))
			})

			t.Run(`range start-end where end exceeds content length`, func(t *ftt.Test) {
				var buf bytes.Buffer
				_, _, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: 40, End: 500}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buf.String(), should.Equal(fullContent[40:]))
			})

			t.Run(`range start-end where start exceeds content length`, func(t *ftt.Test) {
				var buf bytes.Buffer
				_, _, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: 500, End: 600}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buf.Len(), should.Equal(0))
			})

			t.Run(`range start to EOF`, func(t *ftt.Test) {
				var buf bytes.Buffer
				_, _, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: 15, End: -1}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buf.String(), should.Equal(fullContent[15:]))
			})

			t.Run(`suffix range smaller than content`, func(t *ftt.Test) {
				var buf bytes.Buffer
				_, totalSize, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: -1, End: 10}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, totalSize, should.Equal(int64(len(fullContent))))
				assert.Loosely(t, buf.String(), should.Equal(fullContent[len(fullContent)-10:]))
			})

			t.Run(`suffix range larger than content`, func(t *ftt.Test) {
				var buf bytes.Buffer
				_, totalSize, err := FetchHTTPByteRange(ctx, okServer.Client(), okServer.URL, &ByteRange{Start: -1, End: 1000}, &buf)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, totalSize, should.Equal(int64(len(fullContent))))
				assert.Loosely(t, buf.String(), should.Equal(fullContent))
			})

			t.Run(`suffix range with multi-chunk ring buffer wrap`, func(t *ftt.Test) {
				// 100 KiB payload (> 32 KiB readBuf) to exercise multiple reads and ring buffer wrap-around.
				largePayload := make([]byte, 100*1024)
				for i := range largePayload {
					largePayload[i] = byte('a' + (i % 26))
				}
				largeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write(largePayload)
				}))
				defer largeServer.Close()

				// Test suffix smaller than readBuf (100 bytes) and larger than readBuf (50 KiB).
				for _, suffixLen := range []int64{100, 50 * 1024} {
					var buf bytes.Buffer
					_, totalSize, err := FetchHTTPByteRange(ctx, largeServer.Client(), largeServer.URL, &ByteRange{Start: -1, End: suffixLen}, &buf)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, totalSize, should.Equal(int64(len(largePayload))))
					assert.Loosely(t, buf.Bytes(), should.Resemble(largePayload[int64(len(largePayload))-suffixLen:]))
				}
			})
		})
	})

	ftt.Run(`GetCmd flags`, t, func(t *ftt.Test) {
		cmd := GetCmd(base.NewAuthFlags(), ParentTypeWorkUnit)
		run, ok := cmd.CommandRun().(*artifactGetRun)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, run.Flags.Lookup("o"), should.NotBeNil)
		assert.Loosely(t, run.Flags.Lookup("output"), should.NotBeNil)

		unknownCmd := GetCmd(base.NewAuthFlags(), ParentTypeUnknown)
		unknownRun, ok := unknownCmd.CommandRun().(*artifactGetRun)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, unknownRun.Flags.Lookup("invocationid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("workunitid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("testid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("resultid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("artifactid"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("legacy"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("o"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("output"), should.NotBeNil)
		assert.Loosely(t, unknownRun.Flags.Lookup("byte-range"), should.NotBeNil)
	})

	ftt.Run(`artifactGetRun invocation-wide without -workunitid`, t, func(t *ftt.Test) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveHTTPRange(w, r, []byte("artifact payload from get"))
		}))
		defer server.Close()

		t.Run(`fetches single match across root invocation including bare I<digits> and i<digits>`, func(t *ftt.Test) {
			for _, rawInv := range []string{"ants-i05300010637202289", "I05300010637202289", "i05300010637202289"} {
				outFile := filepath.Join(t.TempDir(), "out.txt")
				client := &mockResultDBClient{
					queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
						assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i05300010637202289"))
						return &rdbpb.QueryArtifactsResponse{
							Artifacts: []*rdbpb.Artifact{
								{
									Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log.txt",
									ArtifactId: "host_log.txt",
									FetchUrl:   server.URL,
								},
							},
						}, nil
					},
				}
				run := &artifactGetRun{
					artifactCommonRun: artifactCommonRun{
						client:       client,
						httpClient:   server.Client(),
						parentType:   ParentTypeUnknown,
						invocationID: rawInv,
						artifactID:   "host_log.txt",
						outputFile:   outFile,
					},
				}
				assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
				got, err := os.ReadFile(outFile)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(got), should.Equal("artifact payload from get"))
			}
		})

		t.Run(`fetches via fetch_artifact path prefix when -workunitid is specified`, func(t *ftt.Test) {
			outFile := filepath.Join(t.TempDir(), "out.txt")
			client := &mockResultDBClient{
				getArtifact: func(ctx context.Context, in *rdbpb.GetArtifactRequest) (*rdbpb.Artifact, error) {
					if in.Name == "rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log.txt" {
						return &rdbpb.Artifact{
							Name:       in.Name,
							ArtifactId: "host_log.txt",
							FetchUrl:   server.URL,
						}, nil
					}
					return nil, status.Error(codes.NotFound, "not found")
				},
			}
			run := &artifactGetRun{
				artifactCommonRun: artifactCommonRun{
					client:       client,
					httpClient:   server.Client(),
					parentType:   ParentTypeUnknown,
					invocationID: "I05300010637202289",
					workUnitID:   "ants-wu56200010637202289",
					artifactID:   "/builds/I05300010637202289/12345/target/inv_1/host_log.txt",
					outputFile:   outFile,
				},
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(0))
			got, err := os.ReadFile(outFile)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(got), should.Equal("artifact payload from get"))
		})

		t.Run(`returns exit code 1 when multiple artifacts match`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{
								Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu1/artifacts/logcat.txt",
								ArtifactId: "logcat.txt",
							},
							{
								Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu2/artifacts/logcat.txt",
								ArtifactId: "logcat.txt",
							},
						},
					}, nil
				},
			}
			run := &artifactGetRun{
				artifactCommonRun: artifactCommonRun{
					client:       client,
					httpClient:   server.Client(),
					parentType:   ParentTypeUnknown,
					invocationID: "ants-i05300010637202289",
					artifactID:   "logcat.txt",
				},
			}
			assert.Loosely(t, run.Run(nil, nil, nil), should.Equal(1))
		})

		t.Run(`accepts positional artifact resource name and prints derived IDs`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getArtifact: func(ctx context.Context, in *rdbpb.GetArtifactRequest) (*rdbpb.Artifact, error) {
					return &rdbpb.Artifact{
						Name:       in.Name,
						ArtifactId: "host_log.txt",
						FetchUrl:   server.URL,
					}, nil
				},
			}
			var outBuf bytes.Buffer
			cmd := GetCmd(nil, ParentTypeUnknown)
			run := cmd.CommandRun().(*artifactGetRun)
			run.client = client
			run.httpClient = server.Client()
			run.out = &outBuf
			rc := run.Run(nil, []string{"rootInvocations/ants-i05300010637202289/workUnits/ants-wu56200010637202289/artifacts/host_log.txt"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := outBuf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i05300010637202289"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      ants-wu56200010637202289"))
			assert.Loosely(t, out, should.ContainSubstring("Artifact ID:       host_log.txt"))
			assert.Loosely(t, out, should.ContainSubstring("artifact payload from get"))
		})

		t.Run(`updates derived IDs with canonical artifact ID and fallback work unit on shorthand resolution`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				getArtifact: func(ctx context.Context, in *rdbpb.GetArtifactRequest) (*rdbpb.Artifact, error) {
					return nil, status.Error(codes.NotFound, "not found on parent work unit")
				},
				queryArtifacts: func(ctx context.Context, in *rdbpb.QueryArtifactsRequest) (*rdbpb.QueryArtifactsResponse, error) {
					return &rdbpb.QueryArtifactsResponse{
						Artifacts: []*rdbpb.Artifact{
							{
								Name:       "rootInvocations/ants-i05300010637202289/workUnits/ants-wu-descendant/artifacts/stub%2Fhost_log.txt",
								ArtifactId: "stub/host_log.txt",
								FetchUrl:   server.URL,
							},
						},
					}, nil
				},
			}
			var outBuf bytes.Buffer
			cmd := GetCmd(nil, ParentTypeUnknown)
			run := cmd.CommandRun().(*artifactGetRun)
			run.client = client
			run.httpClient = server.Client()
			run.out = &outBuf
			rc := run.Run(nil, []string{"-artifactid", "host_log.txt", "rootInvocations/ants-i05300010637202289/workUnits/root"}, nil)
			assert.Loosely(t, rc, should.Equal(0))
			out := outBuf.String()
			assert.Loosely(t, out, should.ContainSubstring("Invocation ID:     ants-i05300010637202289"))
			assert.Loosely(t, out, should.ContainSubstring("Work Unit ID:      ants-wu-descendant"))
			assert.Loosely(t, out, should.ContainSubstring("Artifact ID:       stub/host_log.txt"))
			assert.Loosely(t, out, should.ContainSubstring("artifact payload from get"))
		})
	})
}
