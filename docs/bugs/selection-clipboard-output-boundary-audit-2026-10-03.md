# Auditoría adversarial: copiar selección sin salida de terminal — 2026-10-03

## Hallazgo (confirmado y corregido)

`Runtime.CopySelection(clear)` obtenía el payload con `SetClipboard`, pero sólo
escribía la secuencia OSC-52 cuando `rt.Out != nil`. Si `Out` era nil y `clear` era
true, el envío se omitía sin error y el método borraba la selección como si copiar
hubiera sido exitoso. En una sesión SSH sin writer, esto pierde el único contenido que
el usuario podía recuperar de la selección.

## Reproducción y validación

```sh
go test ./internal/engine -run '^TestCopySelectionWithoutOutputDoesNotReportSuccessOrClear$' -count=1
```

## Resolución

- `Runtime.CopySelection` (`internal/engine/terminal.go`) devuelve
  `errClipboardNoWriter` cuando hay una secuencia que emitir y `rt.Out` es nil, y en ese
  caso **no** limpia la selección. El error se comprueba antes de la limpieza, así que
  la ruta previa (writer presente, `clear=true`) conserva su comportamiento.
- `CopySelectionOnRelease` sigue usando `clear=false` e ignorando el error: la selección
  nunca se pierde en esa ruta.
- Se mantiene la llamada a `SetClipboard`, de modo que las rutas nativas/tmux siguen
  intentándose; lo que cambia es que un envío no entregable ya no se reporta como éxito.

### Defecto de instrumento corregido

La condición del fixture estaba invertida: `if err == nil || rt.Selection.HasSelection()`
fallaba precisamente cuando la selección se conservaba, es decir exigía que se borrara,
en contradicción con la expectativa escrita en el informe. Se corrigió a
`!rt.Selection.HasSelection()`.

Prueba de que la prueba corregida sí fija el defecto: contra el código previo (`HEAD`)
vía `go test -overlay` falla con `err=<nil>` y `cleared selection=true`; con el arreglo
pasa (`err` no nil, selección conservada).

## Impacto y límites

- **Tipo:** pérdida de selección por falso éxito en una operación de clipboard.
- **Costura:** la política de ruta (`SetClipboard`), el output opcional de `Runtime`, y
  la limpieza local de `Selection`.
- **Conservador por diseño:** si no hay writer, la operación se reporta como fallo aunque
  una copia nativa asíncrona pueda haber alcanzado el portapapeles local; `SetClipboard`
  documenta que su ruta no confirma éxito, y es preferible no borrar la selección.
- **No demostrado:** pérdida cuando hay un writer disponible; esa ruta escribe la
  secuencia y propaga los errores de escritura, incluido el short write.
- **Confianza:** alta; el caso está determinado sin depender del entorno de clipboard.
