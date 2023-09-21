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

import {
  useEffect, useRef, forwardRef, useImperativeHandle, ForwardedRef,
} from 'react';

import Box from '@mui/material/Box';

import { Ace, Range } from 'ace-builds';
import * as ace from '../ace';
import * as prpc from '../data/prpc';
import * as autocomplete from '../data/autocomplete';


export interface Props {
  requestType: prpc.Message | undefined;
  defaultValue: string;
  readOnly: boolean;
  onInvokeMethod: () => void;
}

export interface RequestEditorRef {
  prepareRequest(): object;
}

export const RequestEditor = forwardRef((
    props: Props, ref: ForwardedRef<RequestEditorRef>) => {
  const editorRef = useRef<ace.AceEditor>(null);

  // Configure the ACE editor on initial load.
  useEffect(() => {
    if (!editorRef.current) {
      return;
    }
    const editor = editorRef.current.editor;

    // Send the request when hitting Shift+Enter.
    editor.commands.addCommand({
      name: 'execute-request',
      bindKey: { win: 'Shift-Enter', mac: 'Shift-Enter' },
      exec: props.onInvokeMethod,
    });

    // Auto-complete JSON property names based on the protobuf schema.
    if (props.requestType) {
      editor.completers = [new Completer(props.requestType)];
      if (editor.getValue() == '{}') {
        editor.focus();
        editor.moveCursorTo(0, 1);
      }
    }
  }, [props.onInvokeMethod, props.requestType]);

  // Expose some methods on the ref assigned to RequestEditor. We do it this
  // way to avoid using AceEditor in a "managed" mode, since it can get slow
  // with large requests.
  useImperativeHandle(ref, () => ({
    prepareRequest: () => {
      if (!editorRef.current) {
        throw new Error('Uninitialized RequestEditor');
      }
      const editor = editorRef.current.editor;
      // Default to an empty request.
      let requestBody = editor.getValue().trim();
      if (!requestBody) {
        requestBody = '{}';
      }
      // The request must be a valid JSON. Verify locally by parsing.
      return JSON.parse(requestBody);
    },
  }));

  return (
    <Box
      component='div'
      sx={{ border: '1px solid #e0e0e0', borderRadius: '2px' }}
    >
      <ace.AceEditor
        ref={editorRef}
        mode={ace.mode}
        theme={ace.theme}
        name='request-editor'
        width='100%'
        height='200px'
        defaultValue={props.defaultValue}
        setOptions={{
          readOnly: props.readOnly,
          useWorker: false,
          tabSize: 2,
          dragEnabled: false,
          showPrintMargin: false,
          enableBasicAutocompletion: true,
          enableLiveAutocompletion: true,
        }}
      />
    </Box>
  );
});

// Eslint wants this.
RequestEditor.displayName = 'RequestEditor';


// JSONType => tokens surrounding values of that type in JSON syntax.
const jsonWrappers = {
  [prpc.JSONType.Object]: ['{', '}'],
  [prpc.JSONType.List]: ['[', ']'],
  [prpc.JSONType.String]: ['"', '"'],
  [prpc.JSONType.Scalar]: ['', ''],
};


// Auto-completes message field names and surrounding syntax.
//
// Returns completions for every field in the message.
const fieldCompletion = (comp: autocomplete.Completion): Ace.Completion[] => {
  return comp.fields.map((field) => {
    // If the value is e.g. a list of strings, we auto-complete `["` and `"]`.
    let leftWrappers = '';
    let rightWrappers = '';
    const typ = field.jsonType;
    leftWrappers += jsonWrappers[typ][0];
    if (typ == prpc.JSONType.List) {
      const elem = field.jsonElementType;
      leftWrappers += jsonWrappers[elem][0];
      rightWrappers += jsonWrappers[elem][1];
    }
    rightWrappers += jsonWrappers[typ][1];
    return {
      snippet: `"${field.jsonName}": ${leftWrappers}\${0}${rightWrappers}`,
      caption: field.jsonName,
      meta: field.type,
      docText: field.doc,
    };
  });
};


// Auto-completes message field names with no extra syntax.
//
// Returns completions only for fields starting with the given prefix.
const fieldCompletionForPrefix = (
    comp: autocomplete.Completion,
    pfx: string): Ace.Completion[] => {
  return comp.fields.
      filter((field) => field.jsonName.startsWith(pfx)).
      map((field) => {
        return {
          value: field.jsonName,
          caption: field.jsonName,
          meta: field.type,
          docText: field.doc,
        };
      });
};


// Auto-completes field values with surrounding syntax.
//
// Returns completions for all possible values of the field if they can be
// enumerated.
const valueCompletion = (
    comp: autocomplete.Completion,
    field: string): Ace.Completion[] => {
  return comp.values(field).map((val) => {
    return {
      value: `"${val.value}"`,
      caption: val.value,
      docText: val.doc,
    };
  });
};


// Auto-completes field values with no extra syntax.
//
// Returns completions only for values starting with the given prefix.
const valueCompletionForPrefix = (
    comp: autocomplete.Completion,
    field: string,
    pfx: string): Ace.Completion[] => {
  return comp.values(field).
      filter((val) => val.value.startsWith(pfx)).
      map((val) => {
        return {
          value: val.value,
          caption: val.value,
          docText: val.doc,
        };
      });
};


// Completer knows how to auto-complete JSON fields based on a protobuf schema.
class Completer implements Ace.Completer {
  readonly id: string = 'prpc-request-completer';
  readonly triggerCharacters: string[] = ['{', '"', ' '];

  constructor(readonly requestType: prpc.Message) {}

  getCompletions(
      _editor: Ace.Editor,
      session: Ace.EditSession,
      pos: Ace.Point,
      _prefix: string,
      callback: Ace.CompleterCallback) {
    // Get all the text before the current position. It is some JSON fragment,
    // e.g. `{"foo": {"bar": [{"ba`. Extract the current JSON path and, perhaps,
    // a partially typed key.
    const ctx = autocomplete.getContext(
        session.getTextRange(new Range(0, 0, pos.row, pos.column)),
    );
    if (ctx == undefined) {
      return;
    }

    // The last path element is what is being edited now. Extract proto message
    // key and value from there (they may be incomplete).
    const last = ctx.path.pop();
    if (!last || last.kind == 'root') {
      return;
    }
    const field = last.key?.val || '';
    const value = last.value?.val || '';

    // All preceding path elements define the parent object of the currently
    // edited field.
    const comp = autocomplete.completionForPath(this.requestType, ctx.path);
    if (comp == undefined) {
      return;
    }

    // Figure out what syntactic element needs to be auto-completed.
    switch (ctx.state) {
      case autocomplete.State.BeforeKey:
        // E.g. `{`. List snippets with all available fields.
        callback(null, fieldCompletion(comp));
        break;
      case autocomplete.State.InsideKey:
        // E.g. `{ "zz`. List only fields matching the typed prefix.
        callback(null, fieldCompletionForPrefix(comp, field));
        break;
      case autocomplete.State.AfterKey:
        // E.g. `{ "zzz"`. Do nothing. Popping auto-complete box for ':' is
        // silly and inserting it unconditionally via ACE API appears to be
        // buggy. Perhaps this can be improved by using "live auto completion"
        // feature.
        break;
      case autocomplete.State.BeforeValue:
        // E.g. `{ "zzz": ` or `[`. List snippets with all possible values.
        callback(null, valueCompletion(comp, field));
        break;
      case autocomplete.State.InsideValue:
        // E.g. `{ "zzz": "xx` or `["xx`. List only values matching the prefix.
        callback(null, valueCompletionForPrefix(comp, field, value));
        break;
      case autocomplete.State.AfterValue:
        // E.g. `{ "zzz": "xxx"` or `["xxx"`. Do nothing. We don't know if this
        // is the end of the message or the user wants to type another field or
        // value.
        break;
    }
  }
}
