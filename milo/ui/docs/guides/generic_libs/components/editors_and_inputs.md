# Editors & inputs components

Rich text editors and specialized input components.

## CodeMirrorEditor

A React wrapper around [CodeMirror 5](https://codemirror.net/5/doc/manual.html).
It handles lifecycle management and solves common configuration issues.

### Usage

```tsx
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import 'codemirror/mode/javascript/javascript'; // Import modes you need

function MyEditor({ code, onChange }) {
  return (
    <CodeMirrorEditor
      value={code}
      initOptions={{
        mode: 'javascript',
        lineNumbers: true,
        readOnly: false,
      }}
      onInit={(editor) => {
        editor.on('change', (instance) => {
          onChange(instance.getValue());
        });
      }}
    />
  );
}
```

### Internal workings

*   **Version**: Uses CodeMirror 5 (imported as `codemirror`).
*   **Initialization**: Initializes the editor instance in a `useEffect` hook.
*   **Updates**: Updates the content via `editor.setValue()` when the `value`
    prop changes.
*   **Refresh Strategy**: Includes a workaround (setTimeout refresh) to ensure
    content renders properly even if initialized early or hidden.
*   **Styling**: Uses `@emotion/styled` for container styling and includes
    necessary CodeMirror CSS.

### Why not other libs?

Code comments indicate why we don't use `@uiw/react-codemirror`:
*   v4 (CodeMirror 6) dropped support for `viewportMargin: Infinity`, which
    breaks searching hidden content.
*   v3 has React-DOM validation errors.
*   Neither offers good support for attaching fold listeners *before* content
    render.

---

## TextAutocomplete

A text box based autocomplete component that provides significantly more
flexibility than MUI's `Autocomplete`.

### Key differences from MUI Autocomplete

### Unified value state
### Function-driven options

### Usage

(See `src/generic_libs/components/text_autocomplete/text_autocomplete.tsx` for
detailed prop definitions)

```tsx
import { TextAutocomplete } from '@/generic_libs/components/text_autocomplete';

<TextAutocomplete
  value={myValue}
  onValueChange={setMyValue}
  items={[
    { label: 'Option 1', value: 'opt1' },
    { label: 'Option 2', value: 'opt2' },
  ]}
  // ... other configuration props
/>
```

### Internal workings

It renders an input field and a popper for suggestions. The logic for showing
suggestions is controlled by the `getOption` prop (or similar mechanism, check
source for exact API) which evaluates the current input and cursor position.
