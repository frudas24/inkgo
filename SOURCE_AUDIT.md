# Source audit notes

The supplied archive was treated as the behavioral source of truth.

Major customized areas observed in the TS fork and represented in the Go design:

- `components/AlternateScreen.tsx`: DEC 1049 lifecycle and mouse tracking
- `components/ScrollBox.tsx`: non-shrinking inner column, sticky scroll, pending delta draining, anchors/clamps
- `screen.ts`: cell screen, wide/spacer cells, style/hyperlink metadata, selection bitmap and diffing
- `output.ts` / `render-node-to-output.ts`: clipping, background/opaque paint, borders, scroll translation and render ordering
- `parse-keypress.ts` / `termio/*`: bracketed paste, CSI-u, modifyOtherKeys, mouse and terminal responses
- `selection.ts` / `searchHighlight.ts`: no-select regions and cell-accurate highlighting
- `focus.ts` / `events/*`: DOM-like capture/bubble interaction
- `cursor.ts` / `use-declared-cursor.ts`: physical cursor declaration for IME/accessibility
- `bidi.ts`: software bidi fallback on terminals without native bidi
- `tabstops.ts`: 8-column tab expansion preserving escape sequences

Missing out-of-tree imports detected in the archive:

- `src/native-ts/yoga-layout/index.ts` (behaviorally important for exact Yoga parity; referenced by the memory regression test, while runtime Yoga imports also target the native layout module)
- `src/bootstrap/state.js`
- `src/utils/debug.js`
- `src/utils/earlyInput.js`
- `src/utils/env.js`
- `src/utils/envUtils.js`
- `src/utils/execFileNoThrow.js`
- `src/utils/fullscreen.js`
- `src/utils/intl.js`
- `src/utils/log.js`
- `src/utils/semver.js`
- `src/utils/sliceAnsi.js`
- `src/utils/tuiTrace.js`
- `src/ink.js` (test-level outer entry point)

The Go implementation intentionally removes the bootstrap/debug/application coupling and uses no JS runtime. The missing helpers were reimplemented where they affect TUI semantics (Unicode/tab handling, terminal capabilities, ANSI slicing/rendering) rather than retained as hidden dependencies.
