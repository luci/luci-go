Supports declaring `<Sticky />` elements that don't get overlapped by
`<Sticky />` elements in a **sibling** `<StickyOffset />`. For example:
```tsx
function Page() {
  return (
    <QueuedStickyScrollingBase>
      <div>content</div>
      <Sticky top>
        <div>first sticky line</div>
      </Sticky>
      <StickyOffset>
        <div>content</div>
        <Sticky top>
          <div>second sticky line</div>
        </Sticky>
        <StickyOffset>
          <div>content</div>
          <Sticky top>
            <div>third sticky line</div>
          </Sticky>
        </StickyOffset>
      </StickyOffset>
      <div css={{height: '200vh'}}>long content</div>
    </QueuedStickyScrollingBase>
  );
}
```
When the page above is scrolled to the bottom, all 3 sticky lines will stick
at the top with appropriate offsets so they are rendered line by line without
overlapping each other.

The offset behavior only applies to `<Sticky />` elements in a sibling
`<StickyOffset />`. For example:
```tsx
function Page() {
  return (
    <QueuedStickyScrollingBase>
      <div>content</div>
      <Sticky top>
        <div>first sticky line</div>
      </Sticky>
      <Sticky top>
        <div>second sticky line</div>
      </Sticky>
      <StickyOffset>
        <div>content</div>
        <Sticky top>
          <div>third sticky line</div>
        </Sticky>
      </StickyOffset>
      <div css={{height: '200vh'}}>long content</div>
    </QueuedStickyScrollingBase>
  );
}
```
When the page above is scrolled to the bottom, the second line will overlap
the first line, while the third line will not overlap the second line.

## Dual-Display Mode

`<Sticky />` supports a "Dual-Display" mode where a component can have an expanded
state in the normal flow and a collapsed state when it becomes sticky. This is
useful for large headers that should take up less space when the user is
scrolling through content.

```tsx
<Sticky
  top
  collapsedContent={<div>Collapsed Header</div>}
>
  <div>Expanded Header (lots of details)</div>
</Sticky>
```

In this mode:
1. The **expanded content** (passed as `children`) scrolls naturally with the page.
2. The **collapsed content** (passed as `collapsedContent`) is hidden while the expanded
   content is visible.
3. Once the expanded content scrolls past the top offset, it disappears and the
   collapsed content becomes sticky at the top.

### Size Reporting

In Dual-Display mode, the component reports a height of `0` to the size recorder
while it is in its expanded state. This ensures that subsequent sticky elements
don't "reserve" space for the header until it actually becomes sticky. Once the
header is stuck (collapsed), it reports its measured height so that following
sticky elements stack below it correctly.

### Layout Caveats

**IMPORTANT**: In Dual-Display mode, the `<Sticky />` component renders **two**
top-level elements: a sticky anchor (height 0) and a relative container for the
normal flow content.

Because of this, parent layouts that expect a single child (e.g., a `flex` container
with `gap`) will see both elements, which may cause unexpected spacing or layout
behavior.

## Performance

`<Sticky />` comes with a rendering performance cost because it needs to track
the size of its children. To avoid this performance cost, CSS variables
`--accumulated-top`, `--accumulated-right`, `--accumulated-bottom`, and
`--accumulated-left` can be used to create non-overlapping sticky elements.
```tsx
function Page() {
  return (
    <QueuedStickyScrollingBase>
      <div>content</div>
      <Sticky top>
        <div>first sticky line</div>
      </Sticky>
      <StickyOffset>
        <div>content</div>
        <div css={{position: 'sticky', top: 'var(--accumulated-top)'}}>
          <div>second sticky line</div>
        </div>
        <StickyOffset>
          <div>content</div>
          <Sticky top>
            <div>third sticky line</div>
          </Sticky>
        </StickyOffset>
      </StickyOffset>
      <div css={{height: '200vh'}}>long content</div>
    </QueuedStickyScrollingBase>
  );
}
```
When the page above is scrolled to the bottom, the second line will not overlap
the first line. However, the third line will overlap the second line. Unlike
`<Sticky />`, sticky elements created with the CSS variables do not ensure that
`<Sticky />` elements in a sibling `<StickyOffset />` don't overlap them.
