// Command bass runs the backendless app state synchronization service.
//
// Subcommands:
//
//	serve     run the HTTP server
//	migrate   run database migrations
//	version   print build information
package main

import (
	"fmt"
	"os"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

const usage = `bass - backendless app state sync

Usage:
  bass <command> [flags]

Commands:
  serve     run the bass HTTP server
  migrate   run database migrations (up by default)
  version   print build information

Run 'bass <command> --help' for command-specific flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "serve":
		os.Exit(runServe(args))
	case "migrate":
		os.Exit(runMigrate(args))
	case "version", "--version", "-v":
		fmt.Printf("bass %s\ncommit: %s\nbuilt:  %s\n", version, commit, date)
		os.Exit(0)
	case "-h", "--help", "help":
		fmt.Print(usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
}
