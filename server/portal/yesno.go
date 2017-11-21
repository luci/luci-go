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

package portal

import (
	"errors"
)

// YesOrNo is a bool that serializes to 'yes' or 'no'.
//
// Useful in Fields that define boolean values.
type YesOrNo bool

// String returns "yes" or "no".
func (yn YesOrNo) String() string {
	if yn {
		return "yes"
	}
	return "no"
}

// Set changes the value of YesOrNo.
func (yn *YesOrNo) Set(v string) error {
	switch v {
	case "yes":
		*yn = true
	case "no":
		*yn = false
	default:
		return errors.New("expecting 'yes' or 'no'")
	}
	return nil
}

// YesOrNoField modifies the field so that it corresponds to YesOrNo value.
//
// It sets 'Type', 'ChoiceVariants' and 'Validator' properties.
func YesOrNoField(f Field) Field {
	f.Type = FieldChoice
	f.ChoiceVariants = []string{"yes", "no"}
	f.Validator = func(v string) error {
		var x YesOrNo
		return x.Set(v)
	}
	return f
}
