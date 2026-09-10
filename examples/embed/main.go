package main

import (
	"fmt"

	layout "github.com/frudas24/inkgo/layout"
	render "github.com/frudas24/inkgo/render"
	widgets "github.com/frudas24/inkgo/widgets"
)

// This example deliberately imports domain packages instead of the root
// package, demonstrating the supported shape for embedding tui-go in a larger
// Go codebase.
func main() {
	status := widgets.Text("ready")
	root := widgets.Root(
		widgets.Box(
			layout.Style{
				FlexDirection: layout.Column,
				Width:         layout.Percent(100),
				PaddingX:      layout.I(1),
			},
			widgets.Text("reopencode", render.TextStyle{Bold: true}),
			status,
		),
	)

	r := render.New(render.RenderOptions{Width: 32, Height: 4})
	fmt.Println(r.Render(root).Screen.PlainText())

	status.SetText("updated without rebuilding the tree")
	fmt.Println(r.Render(root).Screen.PlainText())
}
