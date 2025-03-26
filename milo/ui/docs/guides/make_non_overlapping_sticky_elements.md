# Make non-overlapping sticky elements

When you attempt to make a sticky element in `<body />` using CSS
`position: sticky`, you will find that your element gets overlapped by the top
bar.

A naive solution to this is to add a `top: ${my_hardcoded_size}px` to the
component style. This works well until the size of the top bar changes.

To avoid hard coding, you can use `top: var(--accumulated-top)` instead. This
CSS variable is populated by the `@/generic_libs/components/queued_sticky`
module.

`@/generic_libs/components/queued_sticky` can help you create sticky elements
that don't overlap each other. For more details and advanced usages, please
refer to `@/generic_libs/components/queued_sticky`'s
[documentation](../../src/generic_libs/components/queued_sticky/doc.md).
