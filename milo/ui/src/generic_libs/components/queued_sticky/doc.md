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

`<Sticky />` comes with a rendering performance cost because it needs track the
size of its children. To avoid this performance cost, CSS variables
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
