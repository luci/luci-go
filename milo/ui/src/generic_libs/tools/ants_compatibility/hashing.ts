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

import { hashString } from './murmur';
import { AggregationLevel, InvocationID, TestID } from './types';

const EMPTY_MODULE = 'empty_module_identifier';
const EMPTY_PACKAGE = 'empty_package_identifier';
const EMPTY_CLASS = 'empty_class_identifier';
const EMPTY_METHOD = 'empty_method_identifier';

// Formats a string to make it suitable for hashing.
export function quote(s: string): string {
  return `"${s.replace(/"/g, '\\"')}"`;
}

export interface SerializableValue {
  serialize(): string;
}

export class SimpleValue implements SerializableValue {
  constructor(private readonly value: string) {}
  serialize(): string {
    return quote(this.value);
  }
}

export class KeyValue implements SerializableValue {
  constructor(
    private readonly key: string,
    private readonly value: string,
  ) {}
  serialize(): string {
    return `${quote(this.key)}=${quote(this.value)}`;
  }
}

export class CompoundValue implements SerializableValue {
  private readonly values: SerializableValue[] = [];
  constructor(private readonly name: string) {}

  add(v: SerializableValue): this {
    this.values.push(v);
    return this;
  }

  serialize(): string {
    const parts = this.values.map((v) => v.serialize());
    parts.sort();
    return `${quote(this.name)}={${parts.join(':')}}`;
  }
}

/**
 * TestIdentifierHash returns a result of hashing a test result to generate a consistent test identifier.
 */
export function testIdentifierHash(
  invID: InvocationID,
  testID: TestID,
): string {
  const root = new CompoundValue('test_identifier')
    .add(new KeyValue('branch', invID.branch))
    .add(new KeyValue('target', invID.target))
    .add(new KeyValue('build_provider', invID.buildProvider))
    .add(new KeyValue('scheduler', invID.scheduler))
    .add(new KeyValue('name', invID.testName));

  const properties = new CompoundValue('properties');
  const propKeys = Object.keys(invID.properties || {}).sort();
  for (const k of propKeys) {
    properties.add(new KeyValue(k, (invID.properties || {})[k]));
  }
  root.add(properties);

  // Check whether it is empty module
  if (
    testID.aggregationLevel === AggregationLevel.MODULE &&
    testID.module === '' &&
    Object.keys(testID.moduleParameters || {}).length === 0
  ) {
    root.add(new SimpleValue(EMPTY_MODULE));
  } else {
    root.add(new KeyValue('module', testID.module));
    const moduleParametersCompoundValue = new CompoundValue(
      'module_parameters',
    );
    const paramKeys = Object.keys(testID.moduleParameters || {}).sort();
    for (const k of paramKeys) {
      moduleParametersCompoundValue.add(
        new KeyValue(k, (testID.moduleParameters || {})[k]),
      );
    }
    root.add(moduleParametersCompoundValue);
  }

  if (
    testID.aggregationLevel === AggregationLevel.PACKAGE &&
    testID.testClass === ''
  ) {
    root.add(new SimpleValue(EMPTY_PACKAGE));
  } else if (
    testID.aggregationLevel === AggregationLevel.CLASS &&
    testID.testClass === ''
  ) {
    root.add(new SimpleValue(EMPTY_CLASS));
  } else {
    root.add(new KeyValue('test_class', testID.testClass));
  }

  if (
    (testID.aggregationLevel === AggregationLevel.METHOD ||
      testID.aggregationLevel === AggregationLevel.UNSPECIFIED) &&
    testID.method === ''
  ) {
    root.add(new SimpleValue(EMPTY_METHOD));
  } else {
    root.add(new KeyValue('test_method', testID.method));
  }

  return hashString(root.serialize());
}

/**
 * InvocationHash returns the result of hashing invocation. This creates a consistent identifier for a test configuration.
 */
export function invocationHash(invID: InvocationID): string {
  const root = new CompoundValue('invocation_identifier')
    .add(new KeyValue('branch', invID.branch))
    .add(new KeyValue('target', invID.target))
    .add(new KeyValue('build_provider', invID.buildProvider))
    .add(new KeyValue('scheduler', invID.scheduler))
    .add(new KeyValue('name', invID.testName));

  const properties = new CompoundValue('properties');
  const propKeys = Object.keys(invID.properties || {}).sort();
  for (const k of propKeys) {
    properties.add(new KeyValue(k, (invID.properties || {})[k]));
  }
  root.add(properties);

  return hashString(root.serialize());
}
