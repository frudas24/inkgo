# Auditoría adversarial: escritura parcial de salida del terminal — 2026-10-03

## Hallazgo (confirmado y corregido)

`Runtime.WriteRaw` y `Renderer.WriteFrame` llamaban a `io.WriteString` y descartaban
el número de bytes escritos. Si el `io.Writer` devolvía `n < len(data)` y
`err == nil`, la operación se consideraba exitosa. En ambos flujos, `Renderer.Render`
ya publicó ese frame como su estado previo antes de escribir el patch. Como no se
detectaba el short write, las capas de salida tampoco invalidaban ese historial, y el
siguiente render producía un patch vacío sin reparar la pantalla física.

## Reproducción y validación

```sh
go test ./internal/engine -run '^TestRuntimeRenderDetectsShortWriteAndRetriesFrame$' -count=1
go test ./internal/engine -run '^TestRendererWriteFrameDetectsShortWriteAndRetriesFrame$' -count=1
```

## Resolución

- `Runtime.WriteRaw` (`internal/engine/terminal.go`) y `Renderer.WriteFrame`
  (`internal/engine/renderer.go`) escriben ahora a través de `writeFullString`, que
  normaliza `n != len(s), err == nil` a `io.ErrShortWrite`.
- La ruta de recuperación ya existía: tanto `Runtime.Render` como `Renderer.WriteFrame`
  invalidan el renderer ante cualquier error, de modo que el siguiente frame es un
  redibujo completo en vez de un diff contra un estado físico que nunca se alcanzó.
- Alcance: al centralizar la comprobación en `WriteRaw`, también quedan cubiertas las
  secuencias de lifecycle, título, notificaciones y clipboard.

### Defecto de instrumento corregido

El writer de la prueba embebía `bytes.Buffer`, cuyo `WriteString` promovido satisface
`io.StringWriter`; `io.WriteString` tomaba ese atajo y almacenaba el payload completo,
por lo que la prueba nunca producía una escritura parcial (el mensaje mostraba
`emitted bytes=98`). Se añadió `WriteString` al fixture, que enruta por `Write`, igual
que `regressionWriter` en `internal/engine/terminal_regression_test.go:166`.

Prueba de que la prueba corregida sí fija el defecto: contra el código previo
(`HEAD`) vía `go test -overlay`, ambos casos fallan con `error=<nil>`,
`retry patch bytes=0`, `emitted bytes=49`; con el arreglo pasan.

## Impacto y límites

- **Tipo:** integridad de salida / desincronización entre renderer incremental y
  terminal.
- **Superficies acopladas:** `Runtime.WriteRaw`, `Runtime.Render`,
  `Renderer.WriteFrame` y el historial incremental de `Renderer.Render`. La recuperación
  supone que el llamador pueda reintentar; un short write reiterado por el writer sigue
  devolviendo error y se propaga en vez de silenciarse.
- **No demostrado:** corrupción del árbol o del estado de widgets; el problema observado
  afecta la salida física del terminal.
- **Confianza:** alta; el contrato de `io.Writer` prohíbe `n < len(s)` con error nil.
