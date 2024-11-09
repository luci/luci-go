// Copyright 2019 The LUCI Authors.
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

package host

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/common/types"

	"go.chromium.org/luci/luciexe"
	"go.chromium.org/luci/luciexe/host/buildmerge"
)

// spy represents an active Spy on a Butler.
//
// Its job is to interpret all of the build.proto streams within the Butler into
// a single, merged, 'build.proto' stream on the Butler. All merged protos are
// also delivered to the MergedBuildC channel, which the owner of this spy
// MUST drain as quickly as possible.
//
// If a protocol violation occurs within the Butler run, the spy will mark the
// merged build as INFRA_FAILURE status, and report the error in the build's
// SummaryMarkdown.
type spy struct {
	// MergedBuildC is the channel which sends EVERY merged Build message which
	// this spy produces
	//
	// MergedBuildC will close when the spy is done processing ALL data.
	//
	// The owner of the spy MUST drain this channel as quickly as possible, or
	// it will block the merge build process.
	MergedBuildC <-chan *bbpb.Build

	// Wait on this channel for the spy to drain. Will only drain after calling
	// Close() at least once.
	DrainC <-chan struct{}

	// Close makes the spy stop processing data, and will cause MergedBuildC to
	// close.
	//
	// Safe to call more than once.
	Close func()

	// The namespace under which all user build.proto streams are expected.
	UserNamespace types.StreamName
}

// spyOn installs a Build spy on the Butler.
//
// Monitors '$LOGDOG_NAMESPACE/u/build.proto' datagram stream for Build
// messages, merges them according to the luciexe protocol, and exports the
// merged Build messages to '$LOGDOG_NAMESPACE/build.proto' as well as
// spy.MergedBuildC.
//
// The spy should be Close()'d once the caller is no longer interested in
// receiving merged builds.
//
// Environment: Observes logdog environment variables to determine base values
// for Build.Log.Url and Build.Log.ViewUrl. Accordingly, this relies on the
// Butler's environment already having been exported.
//
// Side-effect: Opens "$LOGDOG_NAMESPACE/build.proto" datagram stream in Butler
//
//	to output merged Build messages.
//
// Side-effect: Exports LOGDOG_NAMESPACE="$LOGDOG_NAMESPACE/u" to the
//
//	environment.
func spyOn(ctx context.Context, b *butler.Butler, base *bbpb.Build) (*spy, error) {
	curNamespace := types.StreamName(os.Getenv(luciexe.LogdogNamespaceEnv))

	ldClient := streamclient.NewLoopback(b, types.StreamName(curNamespace))

	// curNamespace is "$LOGDOG_NAMESPACE"
	// userNamespace is "$LOGDOG_NAMESPACE/u"
	// userNamespaceSlash is "$LOGDOG_NAMESPACE/u/"
	userNamespace := curNamespace.AsNamespace() + "u"
	if err := os.Setenv(luciexe.LogdogNamespaceEnv, string(userNamespace)); err != nil {
		panic(err)
	}
	builds, err := buildmerge.New(ctx, userNamespace, base, mkURLCalcFn())
	if err != nil {
		return nil, err
	}

	fwdChan := teeLogdog(ctx, builds.MergedBuildC, ldClient)

	builds.Attach(b)
	return &spy{
		MergedBuildC:  fwdChan,
		DrainC:        builds.DrainC,
		Close:         builds.Close,
		UserNamespace: types.StreamName(userNamespace).AsNamespace(),
	}, nil
}

// teeLogdog tees Build messages to a new "build.proto" datagram stream on the
// given logdog client.
func teeLogdog(ctx context.Context, in <-chan *bbpb.Build, ldClient *streamclient.Client) <-chan *bbpb.Build {
	out := make(chan *bbpb.Build)

	dgStream, err := ldClient.NewDatagramStream(
		ctx, luciexe.BuildProtoStreamSuffix,
		streamclient.WithContentType(luciexe.BuildProtoZlibContentType))
	if err != nil {
		panic(err)
	}

	go func() {
		defer close(out)
		defer func() {
			if err := dgStream.Close(); err != nil {
				panic(err)
			}
		}()

		// keep buf and z between rounds; this means we should be able to "learn"
		// how to compress build.proto's between rounds, too, since zlib.Reset()
		// keeps the compressor dictionary.
		buf := bytes.Buffer{}
		z := zlib.NewWriter(&buf)
		done := make(chan struct{})

		for build := range in {
			go func() {
				defer func() {
					done <- struct{}{}
				}()
				out <- build
			}()

			buildData, err := proto.Marshal(build)
			if err != nil {
				panic(err)
			}

			buf.Reset()
			z.Reset(&buf)
			if _, err := z.Write(buildData); err != nil {
				panic(err)
			}
			if err := z.Close(); err != nil {
				panic(err)
			}
			if err := dgStream.WriteDatagram(buf.Bytes()); err != nil {
				panic(err)
			}

			<-done
		}
	}()

	return out
}

func mkURLCalcFn() buildmerge.CalcURLFn {
	// TODO(iannucci): This sort of coupling with the environment variables and
	// their interpretation is pretty bad. This should be fixed so that URL
	// generation is an RPC to Butler instead of string assembly by the user.
	host := os.Getenv(bootstrap.EnvCoordinatorHost)

	if strings.HasPrefix(host, "file://") {
		hostSlash := host
		if !strings.HasSuffix(hostSlash, "/") {
			hostSlash += "/"
		}

		viewURLPrefix := filepath.FromSlash(hostSlash)

		return func(ns, streamName types.StreamName) (url string, viewURL string) {
			fullStreamName := string(ns + streamName)
			url = hostSlash + filepath.FromSlash(fullStreamName)
			// TODO(iannucci): actually implement strict types.StreamName -> (url,
			// filesystem) mapping. Currently ':' is a permitted character, which is
			// not legal on Windows file systems. Fortunately stream names must begin
			// with an alnum character, so "." and ".." are illegal stream names.
			viewURL = viewURLPrefix + filepath.ToSlash(fullStreamName)
			return
		}
	}

	project := os.Getenv(bootstrap.EnvStreamProject)
	prefix := os.Getenv(bootstrap.EnvStreamPrefix)

	urlPrefix := fmt.Sprintf("logdog://%s/%s/%s/+/", host, project, prefix)
	viewURLPrefix := fmt.Sprintf("https://%s/logs/%s/%s/+/", host, project, prefix)

	return func(ns, streamName types.StreamName) (url string, viewURL string) {
		fullStreamName := string(ns + streamName)
		url = urlPrefix + fullStreamName
		viewURL = viewURLPrefix + fullStreamName
		return
	}
}
