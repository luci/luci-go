/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../logdog-stream/logdog.ts" />
///<reference path="../luci-operation/operation.ts" />
///<reference path="../luci-sleep-promise/promise.ts" />
///<reference path="../rpc/client.ts" />
///<reference path="fetcher.ts" />
///<reference path="query.ts" />

namespace LogDog {

  /** Stream status entry, as rendered by the view. */
  export type StreamStatusEntry = {name: string; desc: string;};

  /** LoadingState is the current stream loading state. */
  export enum LoadingState {
    /** No loading state (no status, loaded). */
    NONE,
    /** Resolving a glob into the set of streams via query. */
    RESOLVING,
    /** Loading stream content, alternates with RENDERING. */
    LOADING,
    /** Version of LOADING when the stream has been loading for a long time. */
    LOADING_BEEN_A_WHILE,
    /** Rendering loaded stream content. */
    RENDERING,
    /** Error: Attempt to load failed w/ "Unauthenticated". */
    NEEDS_AUTH,
    /** Error: generic loading failure. */
    ERROR,
  }

  /** Registered callbacks used by Model. Implemented by View. */
  export interface ViewBinding {
    pushLogEntries(entries: LogDog.LogEntry[], l: Location): void;
    clearLogEntries(): void;

    updateControls(c: Controls): void;
    locationIsVisible(l: Location): boolean;
  }

  /** Represents control visibility in the View. */
  type Controls = {
    /** Are we completely finished loading stream data? */
    canSplit: boolean;
    /** Are we currently split? */
    split: boolean;
    /** Show the bottom bar? */
    bottom: boolean;
    /** Is the content fully loaded? */
    fullyLoaded: boolean;
    /** If not undefined, link to this URL for the log stream. */
    logStreamUrl: string | undefined;

    /** Text in the status bar. */
    loadingState: LoadingState;
    /** Stream status entries, or null for no status window. */
    streamStatus: StreamStatusEntry[];
  };

  /** A value for the "status-bar" element. */
  type StatusBarValue = {value: string;};

  /**
   * The underlying "logdog-stream-view" Polymer component
   * (see logdog-stream-view.html).
   *
   * View will manipulate the view via modifications to Component.
   */
  type Component = {
    /** Polymer accessor functions. Each member is an element w/ an ID. */
    $: {
      client: luci.PolymerClient; mainView: HTMLElement; buttons: HTMLElement;
      streamStatus: HTMLElement;
      logSplit: HTMLElement;
      logBottom: HTMLElement;
      logEnd: HTMLElement;
      logs: HTMLElement;
    };

    // Polymer properties.
    streams: string[];
    streamLinkUrl: string | undefined;
    mobile: boolean;
    isSplit: boolean;
    metadata: boolean;
    follow: boolean;
    playing: boolean;
    backfill: boolean;

    /** Polymer read-only setter functions. */
    _setStatusBar(v: StatusBarValue|null): void;
    _setCanSplit(v: boolean): void;
    _setIsSplit(v: boolean): void;
    _setShowStreamingControls(v: boolean): void;
    _setStreamStatus(v: StreamStatusEntry[]): void;

    /** Update functions. */
    _updateSplitVisible(v: boolean): void;
    _updateBottomVisible(v: boolean): void;

    /** Special Polymer callback to apply child styles. */
    _polymerAppendChild(child: HTMLElement): void;
  };

  /**
   * View contains the view manipulation logic.
   *
   * It is bound to a Model, which represents the underlying viewer state and
   * data. The Model can interact with View as a ViewBinding.
   *
   * View, in turn, manipulates the actual "logdog-stream-view" Polymer
   * component using a Component reference.
   */
  export class View implements ViewBinding {
    private onScrollHandler =
        () => {
          this.onScroll();
        }

        private scrollTimeoutId: number |
        null = null;
    private model: Model|null = null;
    private renderedLogs = false;

    constructor(readonly comp: Component) {}

    /** Resets and reloads current viewer state. */
    reset() {
      this.detach();

      // Create "onScrollHandler", which just invokes "_onScroll" while bound
      // to "this". We create it here so we can unregister it later, since
      // "bind" returns a modified value.
      window.addEventListener('scroll', this.onScrollHandler);

      this.resetScroll();
      this.renderedLogs = false;

      // Instantiate our view, and install callbacks.
      let profile =
          ((this.comp.mobile) ? Model.MOBILE_PROFILE : Model.DEFAULT_PROFILE);
      this.model =
          new LogDog.Model(new luci.Client(this.comp.$.client), profile, this);
      this.handleStreamsChanged();
    }

    /** Called to detach any resources used by View (Polymer "detach()"). */
    detach() {
      window.removeEventListener('scroll', this.onScrollHandler);
      this.model = null;
    }

    /** Called when a mouse wheel event occurs. */
    handleMouseWheel() {
      this.comp.follow = false;
    }

    /** Called when the split "Down" button is clicked. */
    handleDownClick() {
      if (this.model) {
        this.model.fetchLocation(Location.HEAD, true);
      }
    }

    /** Called when the split "Up" button is clicked. */
    handleUpClick() {
      if (this.model) {
        this.model.fetchLocation(Location.TAIL, true);
      }
    }

    /** Called when the split "Bottom" button is clicked. */
    handleBottomClick() {
      if (this.model) {
        this.model.fetchLocation(Location.BOTTOM, true);
      }
    }

    /** Called when the "streams" property value changes. */
    async handleStreamsChanged() {
      if (!this.model) {
        return;
      }

      await this.model.resolve(this.comp.streams);

      // If we're not on mobile, start with playing state.
      this.comp.playing = (!this.comp.mobile);

      // Perform the initial fetch after resolution.
      if (this.model) {
        this.model.setAutomatic(this.comp.playing);
        this.model.setFetchFromTail(!this.comp.backfill);
        this.model.fetch(false);
      }
    }

    /** Called when the "playing" property value changes. */
    handlePlayingChanged(v: boolean) {
      if (this.model) {
        // If we're playing, begin log fetching.
        this.model.setAutomatic(v);
      }
    }

    /** Called when the "backfill" property value changes. */
    handleBackfillChanged(v: boolean) {
      if (this.model) {
        // If we're backfilling, then we're not tailing.
        this.model.setFetchFromTail(!v);
      }
    }

    /** Called when the "follow" property value changes. */
    handleFollowChanged(v: boolean) {
      if (this.model) {
        if (v) {
          // If follow is toggled on, automatically begin playing.
          this.comp.playing = true;
          this.maybeScrollToFollow();
        }
      }
    }

    /** Called when the "split" button is clicked. */
    handleSplitClicked() {
      if (!this.model) {
        return;
      }

      // After a split, toggle off playing.
      this.model.split();
      this.model.setFetchFromTail(true);
      this.comp.playing = false;
    }

    /** Called when a sign-in event is fired from "google-signin-aware". */
    handleSignin() {
      if (this.model) {
        this.model.notifyAuthenticationChanged();
      }
    }

    /** Clears asynchornous scroll event status. */
    private resetScroll() {
      if (this.scrollTimeoutId !== null) {
        window.clearTimeout(this.scrollTimeoutId);
        this.scrollTimeoutId = null;
      }
    }

    /**
     * Called each time a scroll event is fired. Since this can be really
     * frequent, this will kick off a "scroll handler" in the background at an
     * interval. Multiple scroll events within that interval will only result
     * in one scroll handler invocation.
     */
    private onScroll() {
      if (this.scrollTimeoutId !== null) {
        return;
      }

      window.setTimeout(() => {
        this.handleScrollEvent();
      }, 100);
    }

    private handleScrollEvent() {
      this.resetScroll();

      // Update our button bar position to be relative to the parent's height.
      // TODO: Investigate using CSS or a less manual mathod for this.
      this.adjustToTop(this.comp.$.buttons);
      this.adjustToTop(this.comp.$.streamStatus);
    }

    private adjustToTop(elem: HTMLElement) {
      // Update our button bar position to be relative to the parent's height.
      let pageRect = this.comp.$.mainView.getBoundingClientRect();
      let elemRect = elem.getBoundingClientRect();
      let adjusted = (elem.offsetTop + pageRect.top - elemRect.top);
      if (adjusted < 0) {
        adjusted = 0;
      }
      elem.style.top = String(adjusted);
    }

    private appendMetaLine(root: HTMLElement, key: string, value: string|null) {
      let line = document.createElement('div');
      line.className = 'log-entry-meta-line';

      let keyE = document.createElement('strong');
      keyE.textContent = key;
      line.appendChild(keyE);

      if (value) {
        let e = document.createElement('span');
        e.textContent = value;
        line.appendChild(e);
      }

      root.appendChild(line);
    }

    pushLogEntries(entries: LogDog.LogEntry[], insertion: Location) {
      // Mark that we've rendered logs (show bars now).
      this.renderedLogs = true;

      // Build our log entry chunk.
      let logEntryChunk = document.createElement('div');
      logEntryChunk.className = 'log-entry-chunk';

      let lastLogEntry = logEntryChunk;
      let lines = new Array<string>();

      entries.forEach(le => {
        let text = le.text;
        if (!(text && text.lines)) {
          return;
        }

        // If we're rendering metadata, render an element per log entry.
        if (this.comp.metadata) {
          let entryRow = document.createElement('div');
          entryRow.className = 'log-entry';

          // Metadata column.
          let metadataBlock = document.createElement('div');
          metadataBlock.className = 'log-entry-meta';

          this.appendMetaLine(
              metadataBlock, 'Timestamp:', String(le.timestamp));
          if (le.desc) {
            this.appendMetaLine(metadataBlock, 'Stream:', le.desc.name);
          }
          this.appendMetaLine(metadataBlock, 'Index:', String(le.streamIndex));

          // Log column.
          let logDataBlock = document.createElement('div');
          logDataBlock.className = 'log-entry-content';

          text.lines.forEach(function(line) {
            if (line.value) {
              lines.push(line.value);
            }
          });

          logDataBlock.textContent = lines.join('\n');
          lines.length = 0;

          entryRow.appendChild(metadataBlock);
          entryRow.appendChild(logDataBlock);

          logEntryChunk.appendChild(entryRow);
          lastLogEntry = entryRow;
        } else {
          // Add this to the lines. We'll assign this directly to logEntryChunk
          // after the loop.
          text.lines.forEach(function(line) {
            if (line.value) {
              lines.push(line.value);
            }
          });
        }
      });

      if (!this.comp.metadata) {
        // Only one HTML element: the chunk.
        logEntryChunk.textContent = lines.join('\n');
        lastLogEntry = logEntryChunk;
      }

      // To have styles apply correctly, we need to add it twice, see
      // https://github.com/Polymer/polymer/issues/3100.
      this.comp._polymerAppendChild(logEntryChunk);

      // Add the log entry to the appropriate place.
      let anchor: Element|null;
      let scrollToTop = false;
      let forceScroll = false;
      switch (insertion) {
        case Location.HEAD:
          // PREPEND to "logSplit".
          this.comp.$.logs.insertBefore(logEntryChunk, this.comp.$.logSplit);

          // If we're not split, scroll to the log bottom. Otherwise, scroll to
          // the split.
          anchor = lastLogEntry;
          break;

        case Location.TAIL:
          // APPEND to "logSplit".
          anchor = this.comp.$.logSplit;

          // Identify the element *after* our insertion point and scroll to it.
          // This provides a semblance of stability as we top-insert.
          //
          // As a special case, if the next element is the log bottom, just
          // scroll to the split, since there is no content to stabilize.
          if (anchor.nextElementSibling !== this.comp.$.logBottom) {
            anchor = anchor.nextElementSibling;
          }

          // Insert logs by adding them before the sibling following the log
          // split (append to this.$.logSplit).
          this.comp.$.logs.insertBefore(
              logEntryChunk, this.comp.$.logSplit.nextSibling);

          // When tailing, always scroll to the anchor point.
          scrollToTop = true;
          forceScroll = true;
          break;

        case Location.BOTTOM:
          // PREPEND to "logBottom".
          anchor = this.comp.$.logBottom;
          this.comp.$.logs.insertBefore(logEntryChunk, anchor);
          break;

        default:
          anchor = null;
          break;
      }

      if (anchor) {
        this.maybeScrollToElement(anchor, scrollToTop, forceScroll);
      }
    }

    clearLogEntries() {
      // Remove all current log elements. */
      for (let cur: Element|null = <Element>this.comp.$.logs.firstChild; cur;) {
        let del = cur;
        cur = cur.nextElementSibling;
        if (del.classList && del.classList.contains('log-entry-chunk')) {
          this.comp.$.logs.removeChild(del);
        }
      }
    }

    locationIsVisible(l: Location) {
      let anchor: HTMLElement;
      switch (l) {
        case Location.HEAD:
        case Location.TAIL:
          anchor = this.comp.$.logSplit;
          break;

        case Location.BOTTOM:
          anchor = this.comp.$.logBottom;
          break;

        default:
          return false;
      }
      return this.elementInViewport(anchor);
    }


    updateControls(c: Controls) {
      this.comp._setCanSplit(c.canSplit);
      this.comp._setIsSplit(c.split);
      this.comp._updateSplitVisible(c.split && this.renderedLogs);
      this.comp._updateBottomVisible(c.bottom && this.renderedLogs);
      this.comp.streamLinkUrl = c.logStreamUrl;

      this.comp._setShowStreamingControls(this.renderedLogs && !c.fullyLoaded);
      if (c.fullyLoaded) {
        this.comp.playing = false;
      }

      switch (c.loadingState) {
        case LogDog.LoadingState.RESOLVING:
          this.loadStatusBar('Resolving stream names...');
          break;
        case LogDog.LoadingState.LOADING:
          this.loadStatusBar('Loading streams...');
          break;
        case LogDog.LoadingState.LOADING_BEEN_A_WHILE:
          this.loadStatusBar('Loading streams (has the build crashed?)...');
          break;
        case LogDog.LoadingState.RENDERING:
          this.loadStatusBar('Rendering logs.');
          break;
        case LogDog.LoadingState.NEEDS_AUTH:
          this.loadStatusBar('Not authenticated. Please log in.');
          break;
        case LogDog.LoadingState.ERROR:
          this.loadStatusBar('Error loading streams (see console).');
          break;

        case LogDog.LoadingState.NONE:
        default:
          this.loadStatusBar(null);
          break;
      }

      this.comp._setStreamStatus(c.streamStatus);
    }

    /** Scrolls to the follow anchor point. */
    private maybeScrollToFollow() {
      // Determine our anchor element.
      let e: HTMLElement;
      if (this.comp.isSplit && this.comp.backfill) {
        // Centering on the split element, at the bottom of the page.
        e = this.comp.$.logSplit;
      } else {
        // Scroll to the bottom of the page.
        e = this.comp.$.logEnd;
      }

      this.maybeScrollToElement(e, false, false);
    }

    /**
     * Scrolls to the specified element, centering it at the top or bottom of
     * the view. By default,t his will only happen if "follow" is enabled;
     * however, it can be forced via "force".
     */
    private maybeScrollToElement(
        element: Element, topOfView: boolean, force: boolean) {
      if (this.comp.follow || force) {
        if (topOfView) {
          element.scrollIntoView({
            behavior: 'auto',
            block: 'end',
          });
        } else {
          // Bug? "block: start" doesn't seem to work the same as false.
          element.scrollIntoView(false);
        }
      }
    }

    /**
     * Loads text content into the status bar.
     *
     * If null is passed, the status bar will be cleared. If text is passed, the
     * status bar will become visible with the supplied content.
     */
    private loadStatusBar(v: string|null) {
      let st: StatusBarValue|null = null;
      if (v) {
        st = {
          value: v,
        };
      }
      this.comp._setStatusBar(st);
    }

    private elementInViewport(el: HTMLElement) {
      let top = el.offsetTop;
      let left = el.offsetLeft;
      let width = el.offsetWidth;
      let height = el.offsetHeight;

      while (el.offsetParent) {
        el = <HTMLElement>el.offsetParent;
        top += el.offsetTop;
        left += el.offsetLeft;
      }

      return (
          top < (window.pageYOffset + window.innerHeight) &&
          left < (window.pageXOffset + window.innerWidth) &&
          (top + height) > window.pageYOffset &&
          (left + width) > window.pageXOffset);
    }
  }
}
