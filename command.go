package main

type command interface {
	Name() string
	Run(name string, args []string) error
}
