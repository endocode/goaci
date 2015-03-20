package proj2aci

type CmdFailedError struct {
	Err error
}

func (e CmdFailedError) Error() string {
	return fmt.Sprintf("CmdFailedError: %s", e.Err.Error())
}

type CmdNotFoundError struct {
	Err error
}

func (e CmdNotFoundError) Error() string {
	return fmt.Sprintf("CmdNotFoundError: %s", e.Err.Error())
}

func RunCmdFull(exec string, args, env []string, cwd string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("No args to execute passed")
	}
	prog := exec
	if prog == "" {
		prog, err := exec.LookPath(args[0])
		if err != nil {
			return CmdNotFound{err}
		}
	}
	cmd := exec.Cmd{
		Path:   prog,
		Args:   args,
		Env:    env,
		Dir:    cwd,
		Stdout: stdout,
		Stderr: stderr,
	}
	Debug(`running command: "`, strings.Join(args, `" "`), `"`)
	if err := cmd.Run(); err != nil {
		return CmdFailedError{err}
	}
	return nil
}

func RunCmd(args, env []string, cwd string) error {
	return RunCmdFull("", args, env, cwd, os.Stdout, os.Stderr)
}
