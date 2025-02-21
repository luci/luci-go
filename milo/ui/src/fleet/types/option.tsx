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

/**
 * A single value in a category for an option. For example, if the category is
 * "dut_name", then the list of OptionValues will contain a list of distinct
 * names of duts.
 *
 * The `value` property is used to uniquely identify this option.
 */
export interface OptionValue {
  label: string;
  value: string;
}

/**
 * Type for values in the first category of the multi-select menu. ie: For
 * Fleet Console devices, "dut_name" might be an example of an OptionCategory.
 *
 * The `value` property is used to uniquely identify this category
 */
export type OptionCategory = {
  label: string;
  value: string;
  options: OptionValue[];
};

/**
 * An Object capturing the set of options selected in this filter. The key is
 * the value of the OptionCategory for the selection, and the value is the
 * value for the OptionValue.
 */
export type SelectedOptions = Record<string, string[]>;
