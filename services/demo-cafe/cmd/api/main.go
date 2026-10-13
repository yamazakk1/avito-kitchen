package main

import (
	"os"

	"github.com/yamazakk1/avito-kitchen/services/demo-cafe/internal/app"
)

func main() {
	os.Exit(app.BuildAppFromConfig().Run())
}
