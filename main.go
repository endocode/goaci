package main

import (
	"fmt"
	"os"

	"github.com/appc/goaci/proj2aci"
)

type CmdLineError struct {
	what string
}

func (err *CmdLineError) Error() string {
	return fmt.Sprintf("Command line error: %s", err.what)
}

func newCmdLineError(format string, args ...interface{}) error {
	return &CmdLineError{
		what: fmt.Sprintf(format, args...),
	}
}

func main() {
	if err := mainWithError(); err != nil {
		proj2aci.Warn(err)
		if _, ok := err.(*CmdLineError); ok {
			printUsage()
		}
		os.Exit(1)
	}
}

func mainWithError() error {
	if len(os.Args) < 2 {
		return newCmdLineError("No command specified")
	}
	if c, ok := commandsHash[os.Args[1]]; ok {
		return c.Run(os.Args[2:])
	} else {
		return newCmdLineError("No such command: %q", os.Args[1])
	}
}

func printUsage() {
	proj2aci.Info("blablabla")
}
