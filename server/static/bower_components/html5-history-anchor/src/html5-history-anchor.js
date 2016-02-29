(function() {
  // Extend the <a> tag with the window.history API
  // http://www.whatwg.org/specs/web-apps/current-work/multipage/browsers.html#the-history-interface
  //
  // <a is="html5-history-anchor"
  //   [href="/path"]
  //   [pushstate]
  //   [replacestate]
  //   [back]
  //   [forward]
  //   [go[="0"]]
  //   [title="New Page Title"]
  //   [state="{'message':'New State!'}"]
  //   [popstate]>
  //   title</a>

  var HTML5HistoryAnchorElement = Object.create(HTMLAnchorElement.prototype);

  function historyAnchorEventListener(event) {
    // open in new tab
    if (event.ctrlKey || event.metaKey || event.which === 2) {
      return;
    }

    // pushstate
    if (this.hasAttribute('pushstate')) {
      window.history.pushState(JSON.parse(this.getAttribute('state')), this.getAttribute('title'), this.getAttribute('href'));
      event.preventDefault();
    }

    // replacestate
    if (this.hasAttribute('replacestate')) {
      window.history.replaceState(JSON.parse(this.getAttribute('state')), this.getAttribute('title'), this.getAttribute('href'));
      event.preventDefault();
    }

    // popstate
    if (this.hasAttribute('popstate')) {
      try {
        var popstateEvent = new PopStateEvent('popstate', {
          bubbles: false,
          cancelable: false,
          state: window.history.state
        });

        if ('dispatchEvent_' in window) {
          // FireFox with polyfill
          window.dispatchEvent_(popstateEvent);
        } else {
          // normal
          window.dispatchEvent(popstateEvent);
        }
      } catch(error) {
        // Internet Explorer
        var fallbackEvent = document.createEvent('CustomEvent');
        fallbackEvent.initCustomEvent('popstate', false, false, { state: window.history.state });
        window.dispatchEvent(fallbackEvent);
      }

      event.preventDefault();
    }

    // go
    if (this.hasAttribute('go')) {
      var num = this.getAttribute('go');
      if (num) {
        num = parseInt(num);
      } else {
        num = 0;
      }
      window.history.go(num);
      event.preventDefault();
    }

    // back
    if (this.hasAttribute('back')) {
      window.history.back();
      event.preventDefault();
    }

    // forward
    if (this.hasAttribute('forward')) {
      window.history.forward();
      event.preventDefault();
    }
  }

  HTML5HistoryAnchorElement.createdCallback = function() {
    this.addEventListener('click', historyAnchorEventListener, false);
  };

  document.registerElement('html5-history-anchor', {
    prototype: HTML5HistoryAnchorElement,
    extends: 'a'
  });
})();
