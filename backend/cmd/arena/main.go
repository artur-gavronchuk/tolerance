// Command arena holds the local tools for the tolerance tanks game: scaffold a starter bot and play
// matches against other bots on your own machine. Nothing here talks to the platform; bots are uploaded
// from the site.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

// defaultSiteURL is where the platform lives unless ARENA_URL says otherwise; `arena tanks play` uses it
// for the replay hint.
const defaultSiteURL = "https://tolerance.cc"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "tanks":
		err = runTanks(os.Args[2:], os.Stdout, os.Stderr)
	case "version", "--version":
		fmt.Println("arena", buildVersion())
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "arena:", err)
		os.Exit(1)
	}
}

func buildVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: arena <tanks|version>

  tanks new    scaffold a starter tanks bot: arena tanks new <dir> [--lang python|js]
  tanks play   play a local tanks match: arena tanks play <bot>... [--seed N] [--map NAME] [--ticks N] [--out FILE]
  version      print the version

Upload the bot from the site once it plays well.`)
}
