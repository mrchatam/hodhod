package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mrchatam/hodhod/internal/app"
	"github.com/mrchatam/hodhod/internal/config"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/db/migrate"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(app.HealthCheck())
	}
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-admin" {
		os.Exit(runBootstrapAdmin(os.Args[2:]))
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func runBootstrapAdmin(args []string) int {
	fs := flag.NewFlagSet("bootstrap-admin", flag.ExitOnError)
	username := fs.String("username", "admin", "master admin username")
	password := fs.String("password", "", "master admin password (min 8 chars)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if cfg.RunMigrations {
		if err := migrate.Up(cfg.DatabaseDSN); err != nil {
			fmt.Fprintln(os.Stderr, "migrate:", err)
			return 1
		}
	}
	gdb, err := db.Connect(cfg.DatabaseDSN, cfg.IsDev())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	store := db.NewStore(gdb)
	if err := app.BootstrapAdmin(context.Background(), store, *username, *password); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("bootstrap-admin: ok")
	return 0
}
