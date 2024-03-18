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
 * A file that contains a JS UDF that can be used to filter datastore entities
 * to the ones that are expired. This will be used in conjunction of the
 * dataflow bulk delete datastore entities template.
 * See https://cloud.google.com/dataflow/docs/guides/templates/provided/firestore-bulk-delete.
 */

// ~1.5 years ago from now.
// 1.5 year is the TTL we set for LogStream and LogStreamState entities.
// Note: month is 0 indexed.
const expiredCutoff = Date.UTC(2022, 9 - 1, 1);

/**
 * A UDF that determined whether an entity is expired from its `Created`
 * timestamp property.
 * @returns the input `entityJson` if it has expired, or `null` otherwise.
 */
function isExpired(entityJson) {
  const entity = JSON.parse(entityJson);

  // Dataflow UDF seems to use an old JS engine and optional chaining is not
  // supported.
  const hasCreatedTimestamp = entity.properties &&
    entity.properties.Created &&
    entity.properties.Created.timestampValue;

  // If the timestamp is missing somehow, don't delete the entity.
  if (!hasCreatedTimestamp) {
    return null;
  }

  const createdAt = Date.parse(entity.properties.Created.timestampValue);
  if (createdAt > expiredCutoff) {
    return null;
  }

  return entityJson;
}
