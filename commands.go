package main

var (
	commandsHash map[string]Command = make(map[string]Command)
)

func init() {
	commands := []Command{
		NewCommonCommand(&GoCustomization{}),
		NewCommonCommand(&CmakeCustomization{}),
	}
	for _, c := range commands {
		commandsHash[c.Name()] = c
	}
}
