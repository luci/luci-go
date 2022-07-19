// Copyright 2022 The LUCI Authors.
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

package protowalk

import (
	"fmt"
	"reflect"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldSelector is called once per field per message type per process and
// the result is cached by the type name of this FieldProcessor (i.e.
// reflect.TypeOf to observe the package and local type name of the processor)
// and the full proto message name.
//
// Returns an enum of how this processor wants to handle the provided field.
//
// This function is registered with a corresponding FieldProcessor in
// RegisterFieldProcessor.
type FieldSelector func(field protoreflect.FieldDescriptor) ProcessAttr

// RegisterFieldProcessor registers a new FieldProcessor to allow it to be used
// with protowalk.Fields.
//
// This should be called once per FieldProcessor, per process like:
//
//    func init() {
//      protowalk.RegisterFieldProcessor(&MyFP{}, MyFPFieldSelector)
//    }
//
// Calling RegisterFieldProcessor twice for the same FieldProcessor will panic.
func RegisterFieldProcessor(fp FieldProcessor, selector FieldSelector) {
	fieldProcessorSelectorsMu.Lock()
	defer fieldProcessorSelectorsMu.Unlock()

	t := reflect.TypeOf(fp)
	if fieldProcessorSelectors == nil {
		fieldProcessorSelectors = make(map[reflect.Type]FieldSelector, 10)
	}
	if fieldProcessorSelectors[t] != nil {
		panic(fmt.Sprintf("FieldProcessor %T already registered", fp))
	}
	fieldProcessorSelectors[t] = selector
}

// FieldProcessor allows processing a set of proto message fields in conjunction
// with the package-level Fields() function.
//
// Typically FieldProcessor implementations will apply to fields with particular
// annotations, but a FieldProcessor can technically react to any field(s) that
// it wants to.
type FieldProcessor interface {
	// Process will only be called on fields where the registered FieldSelector
	// function already returned a non-zero ProcessAttr value.
	//
	// Process will never be invoked for a field on a nil message. That is,
	// technically, someMessage.someField is 'unset', even if someMessage is nil.
	// Even if the FieldSelector returned ProccessUnset, it would still not be
	// called on someField.
	//
	// If `applied` == true, `data` will be included in the Results from
	// protowalk.Fields.
	//
	// It is allowed for Process to mutate the value of `field` in `msg`, but
	// mutating other fields is undefined behavior.
	//
	// When processing a given message, an instance of FieldProcessor will have
	// its Process method called sequentially per affected field, interspersed
	// with other FieldProcessors in the same Fields call. For example, if you
	// process a message with FieldProcessors A and B, where A processes evenly-
	// numbered fields, and B processes oddly-numbered fields, the calls would
	// look like:
	//   * B.Process(1)
	//   * A.Process(2)
	//   * B.Process(3)
	//
	// If two processors apply to the same field in a message, they'll be called
	// in the order specified to Fields (i.e. Fields(..., A{}, B{}) would call AS
	// then B, and Fields(..., B{}, A{}) would call B then A).
	Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data ResultData, applied bool)
}
