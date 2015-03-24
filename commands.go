package main

var (
	commandsHash map[string]command = make(map[string]command)
)

func init() {
	commands := []command{
		newBuilderCommand(newGoBuild()),
		newBuilderCommand(newCmakeBuild()),
	}
	for _, c := range commands {
		commandsHash[c.Name()] = c
	}
}
