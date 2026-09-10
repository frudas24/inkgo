# Changelog

## Round 2 — 2026-09-09

- reorganized the public surface into `widgets`, `layout`, `text`, `render`, `input`, `interaction`, `selection`, `terminal`, and `scheduler` domains while preserving canonical root type identity;
- corrected mouse click semantics to activate on release only and never after a drag;
- added multi-click selection, keyboard extension, scroll capture/debt and sticky-follow behavior;
- added X10 mouse, expanded terminal-response parsing, XTVERSION identity and xterm.js duplicate-link suppression;
- added fragment timeout handling for ESC and bracketed paste;
- added terminal query orchestration and DA1 barrier semantics;
- hardened suspend/resume, SIGCONT, resize/reconnect and extended-key mode reassertion;
- added smooth settled scroll draining and xterm.js adaptive wheel policy;
- added native/tmux/OSC52 clipboard behavior;
- added macOS raw TTY support;
- added `Runtime.SetRoot`, `WriteRaw`, `ClearTerminal`, `ClearSearch`, viewport/focus state helpers;
- added dependency-free `ErrorOverview` and shared animation `Clock`;
- expanded parity/regression tests and embedding examples.
