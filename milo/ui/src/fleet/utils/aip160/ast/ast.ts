// Copyright 2026 The LUCI Authors.
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

export type Node =
  | Filter
  | Expression
  | Sequence
  | Factor
  | Term
  | Restriction
  | Comparable
  | Member
  | Value;

// Filter, possibly empty
export interface Filter {
  readonly kind: 'Filter';
  readonly expression: Expression | null;
}

// Expressions may either be a conjunction (AND) of sequences or a simple
// sequence.
//
// Note, the AND is case-sensitive.
//
// Example: `a b AND c AND d`
//
// The expression `(a b) AND c AND d` is equivalent to the example.
export interface Expression {
  readonly kind: 'Expression';
  readonly sequences: readonly Sequence[];
}

// Sequence is composed of one or more whitespace (WS) separated factors.
//
// A sequence expresses a logical relationship between 'factors' where
// the ranking of a filter result may be scored according to the number
// factors that match and other such criteria as the proximity of factors
// to each other within a document.
//
// When filters are used with exact match semantics rather than fuzzy
// match semantics, a sequence is equivalent to AND.
//
// Example: `New York Giants OR Yankees`
//
// The expression `New York (Giants OR Yankees)` is equivalent to the
// example.
export interface Sequence {
  readonly kind: 'Sequence';
  readonly factors: readonly Factor[];
}

// Factors may either be a disjunction (OR) of terms or a simple term.
//
// Note, the OR is case-sensitive.
//
// Example: `a < 10 OR a >= 100`
export interface Factor {
  readonly kind: 'Factor';
  readonly terms: readonly Term[];
}

// Terms may either be unary or simple expressions.
//
// Unary expressions negate the simple expression, either mathematically `-`
// or logically `NOT`. The negation styles may be used interchangeably.
//
// Note, the `NOT` is case-sensitive and must be followed by at least one
// whitespace (WS).
//
// Examples:
// * logical not     : `NOT (a OR b)`
// * alternative not : `-file:".java"`
// * negation        : `-30`
export interface Term {
  readonly kind: 'Term';
  readonly negated: boolean;
  readonly simple: Simple;
}

// Simple expressions may either be a restriction or a nested (composite)
// expression.
export type Simple = Restriction | Composite;

// Composite is just a nested Expression, commonly used to group
// terms or clarify operator precedence. We alias it for clarity.
//
// Example: `(msg.endsWith('world') AND retries < 10)`
export type Composite = Expression;

// Restrictions express a relationship between a comparable value and a
// single argument. When the restriction only specifies a comparable
// without an operator, this is a global restriction.
//
// Note, restrictions are not whitespace sensitive.
//
// Examples:
// * equality         : `package=com.google`
// * inequality       : `msg != 'hello'`
// * greater than     : `1 > 0`
// * greater or equal : `2.5 >= 2.4`
// * less than        : `yesterday < request.time`
// * less or equal    : `experiment.rollout <= cohort(request.user)`
// * has              : `map:key`
// * global           : `prod`
//
// In addition to the global, equality, and ordering operators, filters
// also support the has (`:`) operator. The has operator is unique in
// that it can test for presence or value based on the proto3 type of
// the `comparable` value. The has operator is useful for validating the
// structure and contents of complex values.
export interface Restriction {
  readonly kind: 'Restriction';
  readonly comparable: Comparable;
  readonly comparator: string;
  readonly arg: Arg | null;
}

export type Arg = Comparable | Composite;

// Comparable may either be a member or function. As functions are not currently supported, it is always a member.
export interface Comparable {
  readonly kind: 'Comparable';
  readonly member: Member;
}

// Member expressions are either value or DOT qualified field references.
//
// Example: `expr.type_map.1.type`
export interface Member {
  readonly kind: 'Member';
  readonly value: Value;
  readonly fields: readonly Value[];
}

// Value may either be a TEXT or STRING.
//
// TEXT is a free-form set of characters without whitespace (WS)
// or . (DOT) within it. The text may represent a variable, string,
// number, boolean, or alternative literal value and must be handled
// in a manner consistent with the service's intention.
//
// STRING is a quoted string which may or may not contain a special
// wildcard `*` character at the beginning or end of the string to
// indicate a prefix or suffix-based search within a restriction.
export interface Value {
  readonly kind: 'Value';
  readonly quoted: boolean;
  readonly value: string;
}

export function astToString(node: Node | null): string {
  if (!node) return '';

  switch (node.kind) {
    case 'Filter':
      return `filter{${node.expression ? astToString(node.expression) : ''}}`;
    case 'Expression':
      return `expression{${node.sequences.map(astToString).join(',')}}`;
    case 'Sequence':
      return `sequence{${node.factors.map(astToString).join(',')}}`;
    case 'Factor':
      return `factor{${node.terms.map(astToString).join(',')}}`;
    case 'Term':
      return `term{${node.negated ? '-' : ''}${astToString(node.simple)}}`;
    case 'Restriction':
      const parts = [astToString(node.comparable)];
      if (node.comparator) parts.push(JSON.stringify(node.comparator));
      if (node.arg) parts.push(astToString(node.arg));
      return `restriction{${parts.join(',')}}`;
    case 'Comparable':
      return `comparable{${astToString(node.member)}}`;
    case 'Member':
      const fields =
        node.fields.length > 0
          ? `, {${node.fields.map(astToString).join(',')}}`
          : '';
      return `member{${astToString(node.value)}${fields}}`;
    case 'Value':
      return `value{${node.quoted ? 'quoted,' : ''}${JSON.stringify(node.value)}}`;
  }
}
