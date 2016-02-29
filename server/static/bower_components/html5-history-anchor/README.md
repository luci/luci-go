## &lt;a is="html5-history-anchor"&gt;
> Extend the `<a>` tag with the [HTML5 history API](http://www.w3.org/html/wg/drafts/html/master/browsers.html#the-history-interface)
>
> Fully featured version of the [pushstate-anchor](https://github.com/erikringsmuth/pushstate-anchor)

A link from 1992.
```html
<a href="/home">Home</a>
```

Now using the HTML5 `window.history` API.
```html
<a is="html5-history-anchor" href="/home" pushstate popstate
   title="Home Page" state='{"message":"New State!"}'>Home</a>
```

Clicking this link calls the HTML5 history API.
```js
window.history.pushState({message:'New State!'}, 'Home Page', '/home');
window.dispatchEvent(new PopStateEvent('popstate', {
  bubbles: false,
  cancelable: false,
  state: {message:'New State!'}
}));
```

## Install
[Download](https://github.com/erikringsmuth/html5-history-anchor/archive/master.zip) or run `bower install html5-history-anchor --save`

## Import
```html
<link rel="import" href="/bower_components/html5-history-anchor/html5-history-anchor.html">
or
<script src="/bower_components/html5-history-anchor/html5-history-anchor.js"></script>
```

## API
The API is a direct extension of the `<a>` tag and `window.history`. Examples:

Push a new state with `history.pushState(null, null, '/new-state')` and dispatch a `popstate` event.
```html
<a is="html5-history-anchor" href="/new-state" pushstate popstate>New State</a>
```

Replace the current state with `history.replaceState({message:'Replaced State!'}, null, '/replaced-state')` and don't `popstate`.
```html
<a is="html5-history-anchor" href="/replaced-state"
   replacestate state='{"message":"Replaced State!"}'>Replaced State</a>
```

Back button with `history.back()`.
```html
<a is="html5-history-anchor" back>Back</a>
```

Forward button with `history.forward()`.
```html
<a is="html5-history-anchor" forward>Forward</a>
```

Back 2 pages with `history.go(-2)`.
```html
<a is="html5-history-anchor" go="-2">Back 2 Pages</a>
```

Refresh the page with `history.go(0)`.
```html
<a is="html5-history-anchor" go>Refresh</a>
```

## Notes
The [HTML5 history spec](http://www.w3.org/html/wg/drafts/html/master/browsers.html#the-history-interface) is a bit quirky. `history.pushState()` doesn't dispatch a `popstate` event or load a new page by itself. It was only meant to push state into history. This is an "undo" feature for single page applications. This is why you have to manually dispatch a `popstate` event. Including both `pushstate` and `popstate` attributes on the link will push the new state into history then dispatch a `popstate` event which you can use to load a new page with a router.

- `history.pushState()` and `history.replaceState()` don't dispatch `popstate` events.
- `history.back()`, `history.forward()`, and the browser's back and foward buttons do dispatch `popstate` events.
- `history.go()` and `history.go(0)` do a full page reload and don't dispatch `popstate` events.
- `history.go(-1)` (back 1 page) and `history.go(1)` (forward 1 page) do dispatch `popstate` events.

## Build, Test, and Debug [![Build Status](https://travis-ci.org/erikringsmuth/html5-history-anchor.png?branch=master)](https://travis-ci.org/erikringsmuth/html5-history-anchor)
Source files are under the `src` folder. The build process writes to the root directory. The easiest way to debug is to include the source script rather than the minified HTML import.
```html
<script src="/bower_components/html5-history-anchor/src/html5-history-anchor.js"></script>
```

To build:
- Run `bower install` and `npm install` to install dev dependencies
- Lint, build, and minify code changes with `gulp` (watch with `gulp watch`)
- Start a static content server to run tests (node `http-server` or `python -m SimpleHTTPServer`)
- Run unit tests in the browser (PhantomJS doesn't support Web Components) [http://localhost:8080/tests/SpecRunner.html](http://localhost:8080/tests/SpecRunner.html)
- Manually run functional tests in the browser [http://localhost:8080/tests/functional-test-site/](http://localhost:8080/tests/functional-test-site/)
