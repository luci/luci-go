// Copyright 2024 The LUCI Authors.
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

package pubsub

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Default is a dispatcher installed into the server when using NewModule or
// NewModuleFromFlags.
//
// The module takes care of configuring this dispatcher based on the server
// environment and module's options.
//
// You still need to register your handlers in it using RegisterHandler and
// configure Cloud Pub/Sub subscriptions that push to them.
var Default Dispatcher

// RegisterHandler is a shortcut for Default.RegisterHandler.
func RegisterHandler(id string, h Handler) {
	Default.RegisterHandler(id, h)
}

// RegisterJSONPBHandler is a shortcut for Default.RegisterHandler(JSONPB(...)).
//
// It can be used to register handlers that expect JSONPB-serialized protos of
// a concrete type.
func RegisterJSONPBHandler[T any, TP interface {
	*T
	proto.Message
}](id string, h func(context.Context, Message, TP) error) {
	Default.RegisterHandler(id, JSONPB(h))
}

// RegisterWirePBHandler is a shortcut for Default.RegisterHandler(WirePB(...)).
//
// It can be used to register handlers that expect wirepb-serialized protos of
// a concrete type.
func RegisterWirePBHandler[T any, TP interface {
	*T
	proto.Message
}](id string, h func(context.Context, Message, TP) error) {
	Default.RegisterHandler(id, WirePB(h))
}
