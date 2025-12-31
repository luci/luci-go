# Interaction Components

Components that handle user interactions like keyboard shortcuts, dragging, and
clipboard operations.

## Hotkey

Managed, declarative hotkey binding for React.

### Usage

```tsx
import { Hotkey } from '@/generic_libs/components/hotkey';

function MyComponent() {
  return (
    <Hotkey
      hotkey="ctrl+s, command+s"
      handler={(e) => {
        e.preventDefault();
        save();
      }}
    >
      <div>Press Ctrl+S to save</div>
    </Hotkey>
  );
}
```

### Features

*   **Declarative**: Bindings are managed via component lifecycle. Unmounts
    automatically unbind.
*   **Filtering**: Default filter ignores inputs, selects, and textareas. Custom
    filters can be provided.
*   **Scoped**: While the listener is global (bound to `hotkeys-js`), the
    declarative nature makes it feel scoped to the component's presence.

### Internal workings

Uses `hotkeys-js` library. It registers the handler in `useEffect` and
unregisters it on cleanup. It uses `useLatest` to keep the handler reference
stable without re-binding when the callback changes, ensuring performance.

There is also a Lit version `milo-hotkey` available as `HotkeyElement`.

---

## DragTracker

A Lit element (`<milo-drag-tracker>`) that provides better drag events than
native HTML5 drag-and-drop.

### Usage

```html
<milo-drag-tracker></milo-drag-tracker>
<script>
  const tracker = document.querySelector('milo-drag-tracker');
  tracker.addEventListener('dragstart', (e) => console.log('started'));
  tracker.addEventListener('drag', (e) => {
    console.log('dx:', e.detail.dx, 'dy:', e.detail.dy);
  });
  tracker.addEventListener('dragend', (e) => console.log('ended'));
</script>
```

### Features

*   **delta tracking**: Provides `dx` and `dy` relative to the start position in
    the event detail.
*   **Consistency**: Fires `dragend` consistently when dragging stops (mouseup).
*   **Simplicity**: Doesn't require setting `draggable="true"` or handling the
    complex native DnD API.

### Internal Worksings

It listens for `mousedown` on itself. When triggered, it adds global `mousemove`
and `mouseup` listeners to the `document`. It calculates deltas and dispatches
custom events.

---

## CopyToClipboard

A simple icon button to copy text to the clipboard.

### Usage

```tsx
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';

<CopyToClipboard textToCopy="Hello World" />
// or dynamic
<CopyToClipboard textToCopy={() => "Dynamic Text"} />
```

### Features

*   **Visual Feedback**: Changes icon to a checkmark (`Done`) for 1 second after
    successfully copying.
*   **Dynamic Text**: Supports passing a function to generate text at copy time.

### Internal Workings

Uses `copy-to-clipboard` npm package.
