package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kurodakayn/mpp-kubernetes-smoke/checks"
)

func main() {
	os.Exit(run(os.Args[1:], envMap(os.Environ()), os.Stdout, os.Stderr))
}

func run(args []string, env map[string]string, stdout io.Writer, stderr io.Writer) int {
	config, err := ParseConfig(args, env)
	if err != nil {
		if errors.Is(err, ErrHelp) {
			fmt.Fprint(stdout, usage())
			return 0
		}
		fmt.Fprintln(stderr, err)
		fmt.Fprint(stderr, usage())
		return 2
	}

	reporter := NewReporter(stdout, config.Verbose)
	kubectl := NewKubectl(reporter, config.DryRun)
	http := NewHTTPClient(config.RequestTimeout)
	suite := checks.NewSuite(config.CheckSettings(), kubectl, reporter, http)
	suite.Run()
	return reporter.ExitCode()
}

func envMap(entries []string) map[string]string {
	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		for index, char := range entry {
			if char == '=' {
				env[entry[:index]] = entry[index+1:]
				break
			}
		}
	}
	return env
}
