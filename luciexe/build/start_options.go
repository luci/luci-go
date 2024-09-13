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

package build

import (
	"reflect"

	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

// StartOption is an object which can be passed to the Start function, and
// modifies the behavior of the luciexe/build library.
//
// StartOptions are exclusively constructed from the Opt* functions in this
// package.
//
// StartOptions are all unique per Start (i.e. you can only pass one of a kind
// per option to Start).
type StartOption func(*State)

// OptLogsink allows you to associate a streamclient with the started build.
//
// See `streamclient.New` and `streamclient.NewFake` for how to create a client
// suitable to your needs (note that this includes a local filesystem option).
//
// If a logsink is configured, it will be used as the output destination for the
// go.chromium.org/luci/common/logging library, and will recieve all data
// written via the Loggable interface.
//
// If no logsink is configured, the go.chromium.org/luci/common/logging library
// will be unaffected, and data written to the Loggable interface will go to
// an ioutil.NopWriteCloser.
func OptLogsink(c *streamclient.Client) StartOption {
	return func(s *State) {
		s.logsink = c
	}
}

// OptSend allows you to get a callback when the state of the underlying Build
// changes.
//
// This callback will be called at most as frequently as `rate` allows, up to
// once per Build change, and is called with the version number and a copy of
// Build. Only one outstanding invocation of this callback can occur at once.
//
// If new updates come in while this callback is blocking, they will apply
// silently in the background, and as soon as the callback returns (and rate
// allows), it will be invoked again with the current Build state.
//
// Every modification of the Build state increments the version number by one,
// even if it doesn't result in an invocation of the callback. If your program
// modifies the build state from multiple threads, then the version assignment
// is arbitrary, but if you make 10 parallel changes, you'll see the version
// number jump by 10 (and you may, or may not, observe versions in between).
func OptSend(lim rate.Limit, callback func(int64, *bbpb.Build)) StartOption {
	return func(s *State) {
		var err error
		s.sendCh, err = dispatcher.NewChannel[int64](s.ctx, &dispatcher.Options[int64]{
			QPSLimit: rate.NewLimiter(lim, 1),
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior: &buffer.DropOldestBatch{
					MaxLiveItems: 1,
				},
			},
		}, func(batch *buffer.Batch[int64]) error {
			buildPb, vers := func() (*bbpb.Build, int64) {
				s.buildPbMu.Lock()
				defer s.buildPbMu.Unlock()

				// Technically we don't need atomic here because copyExclusionMu is held
				// in WRITE mode, but atomic.Int64 is cleaner and aligns on 32-bit ports.
				vers := s.buildPbVers.Load()

				if s.buildPbVersSent.Load() >= vers {
					return nil, 0
				}
				s.buildPbVersSent.Store(vers)

				build := proto.Clone(s.buildPb).(*bbpb.Build)

				// now we populate Output.Properties
				if s.topLevelOutput != nil || len(s.outputProperties) != 0 {
					build.Output.Properties = s.topLevelOutput.getStructClone()
					for ns, child := range s.outputProperties {
						st := child.getStructClone()
						if st == nil {
							continue
						}
						if build.Output.Properties == nil {
							build.Output.Properties, _ = structpb.NewStruct(nil)
						}
						build.Output.Properties.Fields[ns] = structpb.NewStructValue(st)
					}
				}

				return build, vers
			}()
			if buildPb == nil {
				return nil
			}

			callback(vers, buildPb)
			return nil
		})

		if err != nil {
			// This can only happen if Options is malformed.
			// Since it's statically computed above, that's not possible (or the tests
			// are also panicing).
			panic(err)
		}
	}
}

// OptParseProperties allows you to parse the build's Input.Properties field as
// JSONPB into the given protobuf message.
//
// Message fields which overlap with property namespaces reserved by
// MakePropertyReader will not be populated (i.e. all property namespaces
// reserved with MakePropertyReader will be removed before parsing into this
// message).
//
// Type mismatches (i.e. parsing a non-numeric string into an int field) will
// report an error and quit the build.
//
// Example:
//
//	msg := &MyOutputMessage{}
//	state, ctx := Start(ctx, inputBuild, OptParseProperties(msg))
//	# `msg` has been populated from inputBuild.InputProperties
func OptParseProperties(msg proto.Message) StartOption {
	return func(s *State) {
		s.topLevelInputProperties = msg
	}
}

// OptStrictInputProperties will cause the build to report an error if data is
// passed via Input.Properties which wasn't parsed into OptParseProperties or
// MakePropertyReader.
func OptStrictInputProperties() StartOption {
	return func(s *State) {
		s.strictParse = true
	}
}

// OptOutputProperties allows you to register a property writer for the
// top-level output properties of the build.
//
// The registered message must not have any fields which conflict with
// a namespace reserved with MakePropertyModifier, or this panics.
//
// This works like MakePropertyModifier, except that it works at the top level
// (i.e. no namespace) and the functions operate directly on the State (i.e.
// they do not take a context).
//
// Usage
//
//	var writer func(*MyMessage)
//	var merger func(*MyMessage)
//
//	// one function may be nil and will be skipped
//	... = Start(, ..., OptOutputProperties(&writer, &merger))
//
// in go2 this can be improved (possibly by making State a generic type):
func OptOutputProperties(writeFnptr, mergeFnptr any) StartOption {
	writer, merger, msgT := getWriteMergerFnValues(false, writeFnptr, mergeFnptr)

	return func(s *State) {
		s.topLevelOutput = &outputPropertyState{msg: msgT.New().Interface()}

		if writer.Kind() == reflect.Func {
			writer.Set(reflect.MakeFunc(writer.Type(), func(args []reflect.Value) []reflect.Value {
				s.excludeCopy(func() bool {
					s.topLevelOutput.set(args[0].Interface().(proto.Message))
					return true
				})
				return nil
			}))
		}

		if merger.Kind() == reflect.Func {
			merger.Set(reflect.MakeFunc(merger.Type(), func(args []reflect.Value) []reflect.Value {
				s.excludeCopy(func() bool {
					s.topLevelOutput.merge(args[0].Interface().(proto.Message))
					return true
				})
				return nil
			}))
		}
	}
}
