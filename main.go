package main

import (
	"fmt"
)

type CmdLineError struct {
	string what
}

func (err CmdLineError) Error() string {
	return fmt.Sprintf("Command line error: %s", err.what)
}

func newCmdLineError(format string, args ...interface{}) {
	return CmdLineError{
		what: fmt.Sprintf(format, args...)
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

func main() {
	if err := mainWithError(); err != nil {
		Warn(err)
		os.Exit(1)
	}
}
