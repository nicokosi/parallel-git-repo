package command

type Echo struct{}

func (command *Echo) Executable() string {
	return "pwd"
}

func (command *Echo) Options() []string {
	return []string{}
}

func (command *Echo) Output(output string, errOutput string, err error) string {
	return output
}