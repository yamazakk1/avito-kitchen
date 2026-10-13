package main

import (
	"os"

	"github.com/yamazakk1/avito-kitchen/services/avito-kitchen/internal/app"
)

func main() {
	os.Exit(app.BuildAppFromConfig().Run())
}
