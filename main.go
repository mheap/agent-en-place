package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mheap/agent-en-place/internal/agent"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	debug := flag.Bool("debug", false, "show Docker build output instead of hiding it")
	rebuild := flag.Bool("rebuild", false, "force rebuilding the Docker image")
	dockerfile := flag.Bool("dockerfile", false, "print the generated Dockerfile and exit")
	miseFile := flag.Bool("mise-file", false, "print the generated mise.toml and exit")
	showVersion := flag.Bool("version", false, "show version information")
	configPath := flag.String("config", "", "path to config file (overrides default config locations)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("agent-en-place version %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: %s <agent>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "run 'agent-en-place --help' for available agents\n")
		os.Exit(1)
	}

	tool := strings.ToLower(args[0])

	cfg := agent.Config{
		Debug:          *debug,
		Rebuild:        *rebuild,
		DockerfileOnly: *dockerfile,
		MiseFileOnly:   *miseFile,
		Tool:           tool,
		ConfigPath:     *configPath,
	}

	if err := agent.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
