# Lit integration

Utilities for bridging React and Lit (Web Components), enabling gradual migration
or hybrid architectures.

## ReactLitElement

A base class to build Web Components using React. It allows you to write the
component logic and rendering in React while exposing it as a standard Custom
Element.

### Usage

1.  **Define React Component**:
    ```tsx
    function MyReactComp({ prop }: { prop: string }) {
      return <div>Hello {prop}</div>;
    }
    ```

2.  **Create Custom Element**:
    ```ts
    import { customElement } from 'lit/decorators.js';
    import {
      ReactLitElement,
    } from '@/generic_libs/components/react_lit_element';

    @customElement('my-element')
    class MyElement extends ReactLitElement {
      static get properties() {
        return { prop: { type: String } };
      }

      prop = '';

      renderReact() {
        return <MyReactComp prop={this.prop} />;
      }
    }
    ```

3.  **Bridge in React**:
    Use `<ReactLitBridge>` to ensure Context propagation works correctly across
    the boundary.
    ```tsx
    import {
      ReactLitBridge,
    } from '@/generic_libs/components/react_lit_element';

    function App() {
      return (
        <MyContext.Provider value="accessable-in-lit">
          <ReactLitBridge>
            <my-element prop="world" />
          </ReactLitBridge>
        </MyContext.Provider>
      );
    }
    ```

### Internal workings

*   **Portals**: Uses `ReactDOM.createPortal` to render the React tree into the
    generic DOM node of the custom element.
*   **Events**: Emits events to a `ReactLitElementTracker` to synchronize
    lifecycles.
*   **Style Injection**: Requires careful handling of Emotion caches if using
    `@emotion/styled` inside Shadow DOM (see `renderReact` notes in source).

---

## MobxExtLitElement

Extension of `MobxLitElement` that adds a safe `addDisposer` mechanism.

### Usage

```ts
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';

class MyMobxElement extends MobxExtLitElement {
  connectedCallback() {
    super.connectedCallback();
    this.addDisposer(someMobxReaction(...));
  }
}
```

### Why?

Native `disconnectedCallback` implies clean up, but often you want to register
cleanup logic *during* connection or operation. `addDisposer` creates a stack of
cleanup functions that are automatically called (in reverse order) when the
element disconnects.
