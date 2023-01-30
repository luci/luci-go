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

package realms

// Attrs are the context of a particular permission check.
//
// Attributes specified here are used as inputs to `conditions` predicates in
// conditional bindings. If a service supports conditional bindings, it must
// document what attributes it passes with each permission it checks.
//
// For example, a service may document that when it checks permission
// "service.entity.create" it supplies an attribute with key
// "service.entity.name" and the value matching the name of the entity being
// created.
//
// One then can create a binding like:
//
//	role: "role/service.entities.creator"
//	principals: ["user:restricted-creator@example.com"]
//	conditions: {
//	  restrict {
//	    attribute: "service.entity.name"
//	    values: ["name1", "name2"]
//	  }
//	}
//
// That binding encodes that "user:restricted-creator@example.com" can create
// entities *only* if they are named "name1" or "name2".
type Attrs map[string]string
