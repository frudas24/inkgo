# API stability

The latest published tag is `v0.1.13`. A development checkout may contain additional changes recorded under `Unreleased`.

## Public surface

Compatibility applies to documented identifiers in:

- `github.com/frudas24/inkgo`
- `/widgets`
- `/layout`
- `/text`
- `/render`
- `/input`
- `/interaction`
- `/selection`
- `/terminal`
- `/scheduler`

`internal/...` is implementation detail and may change freely.

## v0.x policy

Before v1, the project follows semantic-version intent even though Go modules treat `v0` as development:

- patch releases (`v0.1.x`) should not intentionally break documented source compatibility;
- minor releases (`v0.x.0`) may add APIs and may make carefully documented breaking changes when the design cannot reasonably evolve otherwise;
- behavioral bug fixes that restore documented semantics are not considered breaking changes;
- deprecated APIs should remain for at least one minor release when practical.

The root facade is generated from engine exports and CI detects generator drift. Public domain packages are compile-tested together from an external package in `integration/`.

## Ownership contracts

Two contracts are especially important for embedders:

1. mutate a `Node` tree from one application/UI owner rather than concurrently from arbitrary goroutines;
2. `Frame.Screen` is stable unless `BorrowFrameScreen` was explicitly enabled, in which case its storage is renderer-owned and valid only until the next render.
