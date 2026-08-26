package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/codex-tweaks/codex-tweaks/backend/internal/rpc"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print backend version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	server := rpc.NewServer(os.Stdin, os.Stdout)
	if err := server.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
