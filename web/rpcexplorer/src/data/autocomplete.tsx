// Copyright 2023 The LUCI Authors.
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

import * as prpc from './prpc';


// Defines the lexical meaning of a parsed token.
export enum TokenKind {
  // Things like `{`, `:`, etc.
  Punctuation,
  // Complete and correct string: `"something"`.
  String,
  // Incomplete string: `"somethi`.
  IncompleteString,
  // A string that has improper escaping inside.
  BrokenString,
  // Numbers and literals like `null`, `true`, etc.
  EverythingElse,
}


// A lexically significant substring of a JSON document.
export interface Token {
  kind: TokenKind; // e.g. TokenKind.String
  raw: string; // e.g. `"something"`
  val: string; // e.g. `something`
}


// tokenizeJSON calls the callback for every JSON lexical token it visits.
//
// It doesn't care about JSON syntax, only its lexems.
//
// See https://www.rfc-editor.org/rfc/rfc7159#section-2.
export const tokenizeJSON = (text: string, visiter: (tok: Token) => void) => {
  const emit = (kind: TokenKind, raw: string) => {
    let val = '';
    switch (kind) {
      case TokenKind.String:
        try {
          val = JSON.parse(raw);
        } catch {
          kind = TokenKind.BrokenString;
        }
        break;
      case TokenKind.IncompleteString:
        try {
          val = JSON.parse(raw + '"');
        } catch {
          // It is OK, incomplete strings may not be parsable.
        }
        break;
      default:
        val = raw;
    }
    visiter({ kind, raw, val });
  };

  const isWhitespace = (ch: string): boolean => {
    return (
      ch === ' ' ||
      ch === '\t' ||
      ch === '\n' ||
      ch === '\r'
    );
  };
  const isPunctuation = (ch: string): boolean => {
    return (
      ch === '{' ||
      ch === '}' ||
      ch === '[' ||
      ch === ']' ||
      ch === ':' ||
      ch === ','
    );
  };

  let idx = 0;
  while (idx < text.length) {
    const ch = text[idx];

    // Skip whitespace between lexems.
    if (isWhitespace(ch)) {
      idx++;
      continue;
    }

    // Emit punctuation lexems.
    if (isPunctuation(ch)) {
      emit(TokenKind.Punctuation, ch);
      idx++;
      continue;
    }

    // Recognize strings (perhaps escaped). We just need to correctly detect
    // where it ends. Actual deescaping will be done with JSON.parse(...).
    if (ch === '"') {
      let end = idx + 1;
      let kind = TokenKind.IncompleteString;
      while (end < text.length) {
        const ch = text[end];
        if (ch === '\\') {
          end += 2; // skip '\\' itself and one following escaped character
          continue;
        }
        if (ch === '\n' || ch === '\r') {
          break; // an incomplete string
        }
        end++; // include `ch` in the final value
        if (ch === '"') {
          kind = TokenKind.String; // the string is complete now
          break;
        }
      }
      emit(kind, text.substring(idx, end));
      idx = end;
      continue;
    }

    // Recognize other lexems we don't care about (like numbers or keywords).
    let end = idx + 1;
    while (end < text.length) {
      const ch = text[end];
      if (isWhitespace(ch) || isPunctuation(ch)) {
        break;
      }
      end++;
    }
    emit(TokenKind.EverythingElse, text.substring(idx, end));
    idx = end;
  }
};


// Describes at what syntactical position we need to do auto-completion.
export enum State {
  // E.g. `{`.
  BeforeKey,
  // E.g. `{ "zz`.
  InsideKey,
  // E.g. `{ "zzz"`.
  AfterKey,
  // E.g. `{ "zzz": `.
  BeforeValue,
  // E.g. `{ "zzz": "xx`.
  InsideValue,
  // E.g. `{ "zzz": "xxx"`.
  AfterValue,
}


// Outcome of parsing a JSON prefix that describes how to auto-complete it.
export interface Context {
  // Describes at what syntactical position we need to do auto-completion.
  state: State;
  // A path inside the JSON object to the field being auto-completed.
  path: PathItem[];
}


// An element of a JSON field path.
export interface PathItem {
  kind: 'root' | 'obj' | 'list';
  key?: Token; // perhaps incomplete
  value?: Token; // perhaps incomplete
}


// Takes a JSON document prefix (e.g. as it is being typed) and returns details
// of how to do auto-completion in it.
//
// Doesn't really fully check JSON syntax, just recognizes enough of it to
// build the key path. If at some point it gets confused, returns undefined.
export const getContext = (text: string): Context | undefined => {
  class Confused extends Error {}

  let state = State.BeforeValue;
  const path: PathItem[] = [{ kind: 'root' }];

  // State machine that consumes tokens and builds Context.
  type Advance = (tok: Token) => State | undefined;
  const perState: { [key in State]: Advance } = {
    [State.BeforeKey]: (tok) => {
      switch (tok.kind) {
        case TokenKind.IncompleteString:
          path[path.length - 1].key = tok;
          return State.InsideKey;
        case TokenKind.String:
        case TokenKind.EverythingElse:
          path[path.length - 1].key = tok;
          return State.AfterKey;
        default:
          return undefined;
      }
    },

    [State.InsideKey]: () => {
      // InsideKey is a terminal state, there should not be tokens after it.
      return undefined;
    },

    [State.AfterKey]: (tok) => {
      if (tok.kind === TokenKind.Punctuation && tok.val === ':') {
        return State.BeforeValue;
      }
      return undefined;
    },

    [State.BeforeValue]: (tok) => {
      switch (tok.kind) {
        case TokenKind.Punctuation:
          if (tok.val === '{') {
            path.push({ kind: 'obj' });
            return State.BeforeKey;
          }
          if (tok.val === '[') {
            path.push({ kind: 'list' });
            return State.BeforeValue;
          }
          return undefined;
        case TokenKind.String:
        case TokenKind.IncompleteString:
        case TokenKind.EverythingElse:
          path[path.length - 1].value = tok;
          return (
            tok.kind === TokenKind.IncompleteString ?
              State.InsideValue :
              State.AfterValue
          );
        default:
          return undefined;
      }
    },

    [State.InsideValue]: () => {
      // InsideValue is a terminal state, there should not be tokens after it.
      return undefined;
    },

    [State.AfterValue]: (tok) => {
      if (tok.kind === TokenKind.Punctuation) {
        switch (tok.val) {
          case ',':
            switch (path[path.length - 1].kind) {
              case 'obj':
                return State.BeforeKey;
              case 'list':
                return State.BeforeValue;
            }
            return undefined;
          case ']':
          case '}':
          {
            const expect = tok.val === ']' ? 'list' : 'obj';
            const last = path.pop();
            if (!last || last.kind !== expect) {
              return undefined;
            }
            return State.AfterValue;
          }
        }
      }
      return undefined;
    },
  };

  const visiter = (tok: Token) => {
    const next = perState[state](tok);
    if (next === undefined) {
      throw new Confused();
    }
    state = next;
  };

  try {
    tokenizeJSON(text, visiter);
  } catch (err) {
    if (err instanceof Confused) {
      return undefined;
    }
    throw err;
  }

  return { state, path };
};


// Completion knows how to list fields and values of a particular JSON
// object or a list based on a protobuf schema.
//
// It represents completion options at some point in a JSON document. Most often
// it maps to a prpc.Message or a repeated field, but it also supports value
// completion of `map<...>` fields, since they are represented as objects in
// protobuf JSON encoding.
export interface Completion {
  // All known fields of the current object.
  fields: prpc.Field[];
  // Enumeration of *values* of a particular field or a list element.
  values(field: string): Value[];
}


// A possible value of a string-typed JSON field.
export interface Value {
  // Actual value.
  value: string;
  // Documentation string for this value.
  doc: string;
}


// TraversalContext is used internally by completionForPath.
//
// It is essentially a "cursor" inside a protobuf descriptor tree, with methods
// to go deeper.
interface TraversalContext {
  // completion produces the Completion matching this context, if any.
  completion(): Completion | undefined;
  // visitField is used by completionForPath to descend into a field.
  visitField(key: string): TraversalContext | undefined;
  // visitIndex is used by completionForPath to descend into a list element.
  visitIndex(): TraversalContext | undefined;
}


// traverseIfObject creates MessageTraversal if `msg` is represented in JSON by
// an object.
const traverseIfObject = (msg?: prpc.Message): MessageTraversal | undefined => {
  if (msg && msg.jsonType === prpc.JSONType.Object) {
    return new MessageTraversal(msg);
  }
  return undefined;
};


// MessageTraversal implements TraversalContext using prpc.Message as a source.
//
// It can descend into message fields.
class MessageTraversal implements TraversalContext {
  constructor(readonly msg: prpc.Message) {}

  completion(): Completion {
    return {
      fields: this.msg.fields,
      values: (field) => {
        const fieldObj = this.msg.fieldByJsonName(field);
        if (fieldObj && !fieldObj.repeated) {
          return fieldValues(fieldObj);
        }
        return [];
      },
    };
  }

  visitField(key: string): TraversalContext | undefined {
    const field = this.msg.fieldByJsonName(key);
    if (!field) {
      return undefined;
    }
    const inner = field.message;
    if (field.repeated) {
      if (inner?.mapEntry) {
        return new MapTraversal(inner);
      }
      return new ListTraversal(field);
    }
    return traverseIfObject(inner);
  }

  visitIndex(): TraversalContext | undefined {
    // Messages are not lists, can't descend into an indexed element.
    return undefined;
  }
}


// ListTraversal implements TraversalContext using a repeated field as a source.
//
// It can descend into the individual list element.
class ListTraversal implements TraversalContext {
  constructor(readonly field: prpc.Field) {}

  completion(): Completion {
    return {
      fields: [],
      values: () => fieldValues(this.field),
    };
  }

  visitField(): TraversalContext | undefined {
    // Lists have no fields.
    return undefined;
  }

  visitIndex(): TraversalContext | undefined {
    return traverseIfObject(this.field.message);
  }
}


// MapTraversal implements TraversalContext using a map entry as a source.
//
// It can descend into individual map values.
class MapTraversal implements TraversalContext {
  constructor(readonly entry: prpc.Message) {}

  completion(): Completion {
    return {
      // Can't enumerate map<...> keys, they are dynamic.
      fields: [],
      // All entries have the same type, can list possible values based on it.
      values: () => {
        // Note 'value' is a magical constant in map<...> descriptors.
        const fieldObj = this.entry.fieldByJsonName('value');
        if (fieldObj) {
          return fieldValues(fieldObj);
        }
        return [];
      },
    };
  }

  visitField(): TraversalContext | undefined {
    const field = this.entry.fieldByJsonName('value');
    if (!field) {
      return undefined;
    }
    return traverseIfObject(field.message);
  }

  visitIndex(): TraversalContext | undefined {
    // Maps are not lists, can't descend into an indexed element.
    return undefined;
  }
}


// Helper for collecting possible values of a given field.
const fieldValues = (field: prpc.Field) : Value[] => {
  const enumObj = field.enum;
  if (enumObj) {
    return enumObj.values.map((val) => {
      return {
        value: val.name,
        doc: val.doc,
      };
    });
  }
  const msgObj = field.message;
  if (msgObj) {
    const val = msgObj.sampleWellKnownValue();
    return val ? [val] : [];
  }
  return [];
};


// Traverses the protobuf descriptors starting from `root` by following the
// given JSON field path, returning the resulting completion, if any.
export const completionForPath = (
    root: prpc.Message,
    path: PathItem[]): Completion | undefined => {
  let cur: TraversalContext | undefined;
  for (const elem of path) {
    switch (elem.kind) {
      case 'root':
        cur = traverseIfObject(root);
        break;
      case 'obj':
        if (!elem.key) {
          return undefined;
        }
        cur = cur ? cur.visitField(elem.key.val) : undefined;
        break;
      case 'list':
        cur = cur ? cur.visitIndex() : undefined;
        break;
    }
    if (!cur) {
      break;
    }
  }
  return cur ? cur.completion() : undefined;
};
