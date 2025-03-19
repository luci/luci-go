// Copyright 2025 The LUCI Authors.
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
 * Add or update query param with value.
 *
 * @param searchParams existing URLParams
 * @param key query parameter name
 * @param value query parameter value
 * @returns new URLSearchParams instance
 */
export const addOrUpdateQueryParam = (
  searchParams: URLSearchParams,
  key: string,
  value: string,
) => {
  const newParams = new URLSearchParams(searchParams.toString());
  newParams.set(key, value);
  return newParams;
};
