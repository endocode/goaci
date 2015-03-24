package main

import (
	"flag"
	"fmt"

	"github.com/appc/goaci/proj2aci"
)

type build interface {
	Name() string
	SetupParameters(parameters *flag.FlagSet)
	GetBuilderCustomizations() proj2aci.BuilderCustomizations
}

type builderCommand struct {
	b build
}

func newBuilderCommand(build build) command {
	return &builderCommand{
		b: build,
	}
}

func (cmd *builderCommand) Name() string {
	custom := cmd.b.GetBuilderCustomizations()
	return custom.Name()
}

func (cmd *builderCommand) Run(name string, args []string) error {
	parameters := flag.NewFlagSet(name, flag.ExitOnError)
	cmd.b.SetupParameters(parameters)
	if err := parameters.Parse(args); err != nil {
		return err
	}
	if len(parameters.Args()) != 1 {
		return fmt.Errorf("Expected exactly one project to build, got %d", len(args))
	}
	custom := cmd.b.GetBuilderCustomizations()
	custom.GetCommonConfiguration().Project = parameters.Args()[0]
	builder := proj2aci.NewBuilder(custom)
	return builder.Run()
}
