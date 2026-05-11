package main

import (
	"log"
	"os"

	"github.com/rivo/tview"
)

func main() {
	if len(os.Args) > 1 {
		runCLI()
		return
	}

	app := &App{}
	var err error
	app.db, err = initDB()
	if err != nil {
		log.Fatal(err)
	}
	defer app.db.Close()

	app.app = tview.NewApplication()
	app.setupUI()
	if err := app.app.SetRoot(app.pages, true).EnableMouse(true).Run(); err != nil {
		log.Fatal(err)
	}
}
