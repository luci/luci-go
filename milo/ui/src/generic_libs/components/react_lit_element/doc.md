# ReactLitElement
## Feature
This module helps to build web component using React.

### Create a web component
You can extend the ReactLitElement to create a web component.
```tsx
@customElement('my-custom-element')
class MyCustomElement extends ReactLitElement {
  static get properties() {
    return {
      myProp: {
        attribute: 'my-prop',
        type: String;
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

  // Component created by this method is rendered as
  // * the Light DOM child of this custom element in the real DOM tree.
  // * a direct child of the nearest ancestor `<ReactLitBridge />` in the React
  //   virtual DOM tree.
  //
  // The event and context propagation follows the propagation rules in their
  // respective DOM tree.
  renderReact() {
    // You can also inline you component here.
    // Hook can be used. Even though eslint may warn you that hooks can only
    // be used in components.
    //
    // It's still recommended to use a standalone component so linters are not
    // confused by this.
    return <MyCustomComponent myProp={this.myProp} />
  }
}

// Declare the custom element in the JSX namespace if you want to use it in
// React template. Otherwise its unnecessary.
declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'my-custom-element': {
        readonly 'my-prop': string;
      };
    }
  }
}

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
