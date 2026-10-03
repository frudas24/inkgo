# Auditorías de bugs

| Informe | Hallazgo | Estado |
|---|---|---|
| [Escritura parcial de salida del terminal — 2026-10-03](terminal-short-write-audit-2026-10-03.md) | `Runtime.Render` y `Renderer.WriteFrame` aceptan `n < len(data), err == nil`; el historial avanza y una pantalla incompleta puede no volver a pintarse. | Corregido |
| [Copia de selección sin writer — 2026-10-03](selection-clipboard-output-boundary-audit-2026-10-03.md) | `CopySelection(true)` omite OSC-52 cuando no hay writer, devuelve éxito y borra la selección. | Corregido |

Ambas pruebas de reproducción tenían además fallos de instrumento, corregidos en el
mismo cambio; cada informe documenta cómo se validó que la prueba sí falla sin el
arreglo (overlay de Go contra el código previo en `HEAD`).
