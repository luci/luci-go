# ReactLitElement
## Feature
This module helps to build web component using React.

### Create a web component
You can extend the ReactLitElement to create a web component.

First, build your component in React like a normal React component.
```tsx
interface MyCustomComponentProps {
  readonly myProp: string;
}

function MyCustomComponent({myProp}: MyCustomComponentProps) {
  // Only context available to the nearest <ReactLitBridge /> is available here.
  const myCtx = useMyContext();

  return (
    <div>
      <div>
        {myProp}
      </div>
      <div>
        {myCtx}
      </div>
    </div>
  )
}
```

Then, create a Lit binding for the component. Typically, this is placed in a
separate sibling file (e.g. `./lit_binding.tsx`). Splitting the binding into a
separate file ensures [React fast refresh](https://reactnative.dev/docs/fast-refresh#how-it-works)
works.
```tsx
import createCache, { EmotionCache } from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { customElement } from 'lit/decorators.js';

import { ReactLitElement } from '@/generic_libs/components/react_lit_element';

@customElement('my-custom-element')
class MyCustomElement extends ReactLitElement {
  static get properties() {
    return {
      myProp: {
        attribute: 'my-prop',
        type: String,
      }
    }
  }

  // Due to our unique tsconfig.json setting combinations, the `@property()`
  // decorator from `lit` doesn't work. The getter/setter pattern is only
  // necessary because of this. It has nothing to do with `ReactLitElement`.
  private _myProp = '';
  get myProp() {
    return this._myProp;
  }
  set myProp(newVal: string) {
    if (this._myProp === newVal) {
      return;
    }
    const oldVal = this._myProp;
    this._myProp = newVal;
    this.requestUpdate('myProp', oldVal);
  }

  private cache: EmotionCache | undefined = undefined;

  // Component created by this method is rendered as
  // * the Light DOM child of this custom element in the real DOM tree.
  // * a direct child of the nearest ancestor `<ReactLitBridge />` in the React
  //   virtual DOM tree.
  //
  // The event and context propagation follows the propagation rules in their
  // respective DOM tree.
  renderReact() {
    // When the Lit binding is used, it is very likely that this element is in a
    // a shadow DOM. Most CSS styles do not propagate through shadow DOM
    // boundaries. Provide emotion cache at this level so the CSS styles are
    // applied correctly.
    // This is not needed if you don't use this web component in a shadow DOM.
    // e.g. when you only use this component to support custom HTML tags
    // rendered in a light DOM.
    if (!this.cache) {
      this.cache = createCache({
        key: 'my-custom-element',
        container: this,
      });
    }

    // You can also inline you component here.
    // Hook can be used. Even though eslint may warn you that hooks can only
    // be used in components.
    //
    // It's still recommended to use a standalone component so linters are not
    // confused by this.
    return (
      <CacheProvider value={this.cache}>
        <MyCustomComponent myProp={this.myProp} />
      </CacheProvider>
    )
  }
}

// Declare the custom element in the JSX namespace if you want to use it in
// React template. Otherwise its unnecessary.
declare module 'react' {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'my-custom-element': {
        readonly 'my-prop': string;
      };
    }
  }
}
```

### Use the web component
You can use the web-component in your HTML under a `<MyContextProvider />`.
```tsx
function Page() {
  // `<my-custom-element />` must be a descendant of a `<ReactLitBridge />`.
  // It does not need to be direct child.
  // However, it's still recommended to place `<my-custom-element />` as close
  // to a `<ReactLitBridge />` as possible. This is because only React context
  // available to that `<ReactLitBridge />` is available to
  // `<my-custom-element />`. Placing them closely reduces confusion around
  // what contexts are available.
  return (
    <MyContextProvider value="used-context">
      <ReactLitBridge>
        <MyContextProvider value="discarded-context">
          <my-custom-element my-prop="prop1" />
        </MyContextProvider>
      </ReactLitBridge>
    </MyContextProvider>
  )
}
```

### Performance
There's a small overhead (due to DOM event propagation) when a ReactLitElement
instance is added or removed to the DOM tree. Other than that, the performance
characteristic should be comparable to a regular React component.

## Q&A
### Why is the CSS style not applied correctly?
Most CSS style does not propagate through the shadow DOM boundary. To ensure the
styles are applied to the shadow DOMs correctly, the style sheets need to be
injected to the shadow/light DOM where the element is rendered at.

In most cases, you can apply the style to the shadow root using the following
approach.
```tsx
@customElement('my-custom-element')
class MyCustomElement extends ReactLitElement {
  ...

  private cache: EmotionCache | undefined = undefined;

  renderReact() {
    if (!this.cache) {
      this.cache = createCache({
        key: 'my-custom-element',
        // This instructs emotion to inject the style under this component.
        // As a result, if this component is used under a shadow root, then that
        // shadow root will have the style sheets required to render this
        // component.
        container: this,
      });
    }

    return (
      <CacheProvider value={this.cache}>
        <MyCustomComponent myProp={this.myProp} />
      </CacheProvider>
    )
  }
}
```

However, the approach above does not work when the component renders HTML to
different parts of the DOM tree. This usually happens when tooltips, modals,
snackbar, etc are used. In such case, you need to ensure each segment has their
stylesheet rendered to their nearliest shadow root.

The exact solution depends on the specific case. Some common approaches are:

1. For each HTML section, wrap them in a Lit component with their own emotion
   cache so each HTML section has self-contained CSS styles.
2. Do not render HTML to different parts of the DOM tree. For example, use the
   `disablePortal` option when using a MUI dialog.
3. Apply the emotion cache to the portion of the HTML rendered in the shadow DOM
   only.

```tsx
interface MyTooltipComponentProps {
  readonly cache?: EmotionCache;
}

function MyTooltipComponent({
  // Instead of wrapping the entire component in a cache provider, pass the
  // cache from the parent to this component. This allows us to wrap only a
  // portion of the HTML under a cache provider.
  cache = createCache({
    key: 'my-tooltip-component-react',
    container: document.body,
  }),
}) {
  return (
    <Tooltip
      title={
        <span>
          This is always rendered in the light DOM under document.body.
          It does not need it's own emotion cache.
        </span>
      }
    >
      <span>
        {/* Only wrap the portion that are rendered in the shadow DOM in a cache
         ** provider.
         */}
        <CacheProvider value={cache}>
          <span>
            This might be rendered in a shadow DOM when used in a web component.
            Therefore it needs its own emotion cache.
          </span>
        </CacheProvider>
      </span>
    </Tooltip>
  )
}
```

## Internal
Internally there are 4 pieces that keep this together.

### ReactLitElement
ReactLitElement mainly does four things:
1. Provides a rendering node (itself) to a [React portal](https://react.dev/reference/react-dom/createPortal).
2. Provides a rendering method (`ReactLitElement.renderReact`) that creates a `ReactNode` to be rendered via a portal.
3. When connected/disconnected from DOM, emits events to notify the nearest ancestor `<react-lit-element-tracker />`
   that the set of `ReactLitElement` has changed, so the `<react-lit-element-tracker />` can act accordingly.
4. When there's an update event on the `LitElement` side, trigger a rerender on the React component created by
   `.renderReact`.

### ReactLitElementTrackerElement (`<react-lit-element-tracker />`)
ReactLitElementTrackerElement does two things:
1. Keeps track of all the `ReactLitElement` in its descendent by listening to events emitted by `ReactLitElement`s.
2. When there's a change in `ReactLitElement`, emits an event notify its parent `<ReactLitBridge />` that the set of
   `ReactLitElement` has changed, so the parent `<ReactLitBridge />` can act accordingly.

### ReactLitBridge
ReactLitElementTrackerElement does two things:
1. Renders all the `ReactLitElement` tracked by `<react-lit-element-tracker />` via `<ReactLitRenderer />`.
2. Triggers a rerender when the set of `ReactLitElement` has changed.

### ReactLitRenderer
ReactLitRenderer does one thing:
1. Render a `ReactLitElement` by rendering the `ReactNode` created by the `renderReact` method of the `ReactLitElement`,
   to the `ReactLitElement` itself, via [createPortal](https://react.dev/reference/react-dom/createPortal).
