// Copyright 2026 The LUCI Authors.
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

export const DEFAULT_FLEET_ROUTE = '/ui/fleet/p/chromeos/devices';
const HOST_PARAMS = new Set(['fcon_data', 'fcon_env']);
const LINK_EVENTS = ['click', 'auxclick', 'contextmenu', 'mousedown'];
type NavWin = typeof window & { __fconNavClean?: () => void };

export function normalizePrototypeRoute(raw?: string | null): string | null {
  const routeStr = String(raw ?? '').trim();
  if (!/^#?(?:\/?ui\/|\/?fleet(?:\/|$))/.test(routeStr)) return null;
  return routeStr
    .replace(/^#?\/?fleet(?=\/|$)/, '/ui/fleet')
    .replace(/^#?\/?ui\//, '/ui/')
    .replace(/^\/ui\/fleet\/labs\/home\b/, '/ui/fleet')
    .replace(/^\/ui\/fleet\/labs\/devices\b/, DEFAULT_FLEET_ROUTE)
    .replace(/^\/ui\/fleet\/labs\//, '/ui/fleet/');
}
export function rewritePrototypeHref(raw: string, base?: string): string {
  const direct = normalizePrototypeRoute(raw);
  if (direct) return '#' + direct;
  if (!/^(?:https?:)?\/\//i.test(String(raw ?? '').trim())) return raw;
  try {
    const baseObj = new URL(base ?? location.href);
    const url = new URL(raw, baseObj.href);
    if (url.origin !== baseObj.origin) return raw;
    const route = normalizePrototypeRoute(url.pathname + url.search + url.hash);
    if (route) return '#' + route;
    const samePath = url.pathname === baseObj.pathname;
    const hashRoute = samePath && normalizePrototypeRoute(url.hash);
    const queryUrl = url.origin + url.pathname + url.search;
    const prefix = url.search !== baseObj.search ? queryUrl : '';
    return hashRoute ? `${prefix}#${hashRoute}` : raw;
  } catch {
    return raw;
  }
}
export function rewritePrototypeNavigationUrl(url: string | URL, b?: string) {
  const nextHref = rewritePrototypeHref(String(url), b);
  if (!nextHref.startsWith('#/ui/')) return nextHref;
  const baseUrl = new URL(b ?? location.href);
  return `${baseUrl.origin}${baseUrl.pathname}${baseUrl.search}${nextHref}`;
}
export function resolveInitialPrototypeUrl(href: string, def = '', cfg = '') {
  const fallback = normalizePrototypeRoute(def) ?? DEFAULT_FLEET_ROUTE;
  try {
    const hostUrl = new URL(href);
    const params = hostUrl.searchParams;
    const routeParam = params.get('route') ?? params.get('target_route');
    const cands = [hostUrl.hash, routeParam, cfg];
    const initRoute = cands.map(normalizePrototypeRoute).find(Boolean);
    const routeUrl = new URL(initRoute ?? fallback, 'http://localhost');
    for (const [key, val] of Array.from(params.entries())) {
      if (HOST_PARAMS.has(key)) continue;
      if (!/^(?:target_)?route$/.test(key) && !routeUrl.searchParams.has(key)) {
        routeUrl.searchParams.append(key, val);
      }
      params.delete(key);
    }
    const route = routeUrl.pathname + routeUrl.search + routeUrl.hash;
    hostUrl.hash = '#' + route;
    return { route, cleanHref: hostUrl.toString() };
  } catch {
    return { route: fallback, cleanHref: href };
  }
}
export function installPrototypeNavigationInterceptor(w: Window = window) {
  const navWin = w as NavWin;
  if (navWin.__fconNavClean) return navWin.__fconNavClean;
  const { document: doc, history: hist, location: loc, open: origOpen } = w;
  const elemProto = (navWin.Element ?? Element).prototype;
  const origSetAttr = elemProto.setAttribute;
  const [origPush, origRep] = [hist.pushState, hist.replaceState];
  const rewriteHref = (val: string) => rewritePrototypeHref(val, loc.href);
  const rewriteAnchor = (el: Element | null) => {
    const href = el?.getAttribute('href');
    if (el && href) el.setAttribute('href', href);
  };
  elemProto.setAttribute = function (name: string, val: string) {
    const isAnchorHref = this.tagName === 'A' && name.toLowerCase() === 'href';
    origSetAttr.call(this, name, isAnchorHref ? rewriteHref(val) : val);
  };
  const onLinkEvent = (ev: Event) => {
    if (ev.target instanceof Element) rewriteAnchor(ev.target.closest('a'));
  };
  const onHashChange = () => {
    const normHash = normalizePrototypeRoute(loc.hash);
    if (normHash && loc.hash !== '#' + normHash) loc.replace('#' + normHash);
  };
  LINK_EVENTS.forEach((ev) => doc.addEventListener(ev, onLinkEvent, true));
  w.addEventListener('hashchange', onHashChange);
  doc.querySelectorAll('a[href]').forEach(rewriteAnchor);
  const nav = rewritePrototypeNavigationUrl;
  const toUrl = (u?: string | URL | null) => (u ? nav(u, loc.href) : u);
  w.open = (u, t, f) => origOpen.call(w, toUrl(u) ?? undefined, t, f);
  hist.pushState = (d, u, url) => origPush.call(hist, d, u, toUrl(url));
  hist.replaceState = (d, u, url) => origRep.call(hist, d, u, toUrl(url));
  return (navWin.__fconNavClean = () => {
    [elemProto.setAttribute, w.open] = [origSetAttr, origOpen];
    [hist.pushState, hist.replaceState] = [origPush, origRep];
    LINK_EVENTS.forEach((ev) => doc.removeEventListener(ev, onLinkEvent, true));
    w.removeEventListener('hashchange', onHashChange);
    delete navWin.__fconNavClean;
  });
}
