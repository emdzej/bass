package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/emdzej/bass/internal/storage"
)

func runMigrate(args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dbPath := fs.String("db", envOr("BASS_DB_PATH", "bass.db"), "path to the SQLite database")
	_ = fs.Parse(args)

	db, err := storage.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		return 1
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		return 1
	}
	fmt.Println("migrations applied")
	return 0
}
