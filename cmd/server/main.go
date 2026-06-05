package main

import (
	"log"
	"os"

	"github.com/mrchatam/hodhod/internal/app"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(app.HealthCheck())
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
