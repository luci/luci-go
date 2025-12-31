# Observers

Tools for DOM observation patterns, primarily `IntersectionObserver`.

## ObserverElement

A set of decorators and interfaces to build "Observer Aware" Lit elements.
Commonly used for **Lazy Rendering** (render placeholder until visible) or
**Lazy Loading** (fetch data when visible).

### Usage

```ts
import { observer, lazyRendering } from '@/generic_libs/tools/observer_element';

// Method 1: @observer decorator
@customElement('my-element')
@observer
class MyElement extends MobxLitElement implements ObserverElement {
  notify() {
    console.log('I am visible!');
  }
}

// Method 2: @lazyRendering decorator (Simplifies lazy rendering pattern)
@customElement('lazy-element')
@lazyRendering
class LazyElement extends MobxLitElement implements RenderPlaceHolder {
  renderPlaceHolder() {
    return html`Loading...`;
  }
  render() {
    return html`Actual Heavy Content`;
  }
}
```

### Internal workings

*   **`IntersectionNotifier`**: A shared `IntersectionObserver` single to reduce
    overhead.
*   **`ProgressiveNotifier`**: Dispatches notifications in batches (e.g., 10
    elements every 100ms) to avoid jank when many observers trigger
    simultaneously.
*   **`@observer`**: Mixin that automatically subscribes the element to the
    notifier when connected and unsubscribes when disconnected.
