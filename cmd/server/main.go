package main

import (
	"log"

	"github.com/mrchatam/hodhod/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
