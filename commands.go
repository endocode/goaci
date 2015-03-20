package main

var (
	commandGo = &CommonCommand{
		name: "go",
	}
	commandCmake = &CommonCommand{
		name: "cmake",
	}
	commandAutotools = &CommonCommand{
		name: "autotools",
	}
	commandsHash map[string]*Command

func init() {
	commands := []*Command{commandGo, commandCmake, commandAutotools}
	for _, c := range commands {
		commandsHash[c.Name] = c
	}
}
