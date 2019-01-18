package commands

import (
	"github.com/concourse/concourse/atc"
	"github.com/concourse/concourse/fly/commands/internal/hijacker"
	"github.com/concourse/concourse/fly/pty"
	"github.com/concourse/concourse/fly/rc"
	"github.com/tedsuo/rata"
	"io"
	"os"
)

type ShellCommand struct {
	PositionalArgs struct {
		Command []string `positional-arg-name:"command" description:"The command to run in the container (default: bash)"`
	} `positional-args:"yes"`
}

func (command *ShellCommand) Execute([]string) error {
	target, err := rc.LoadTarget(Fly.Target, Fly.Verbose)
	if err != nil {
		return err
	}

	err = target.Validate()
	if err != nil {
		return err
	}

	shellContainer, err := target.Team().CreateShell()
	if err != nil {
		return err
	}

	path, args := remoteCommand(command.PositionalArgs.Command)

	reqGenerator := rata.NewRequestGenerator(target.URL(), atc.Routes)

	var ttySpec *atc.HijackTTYSpec
	rows, cols, err := pty.Getsize(os.Stdout)
	if err == nil {
		ttySpec = &atc.HijackTTYSpec{
			WindowSize: atc.HijackWindowSize{
				Columns: cols,
				Rows:    rows,
			},
		}
	}

	spec := atc.HijackProcessSpec{
		Path: path,
		Args: args,
		Env:  []string{"TERM=" + os.Getenv("TERM")},
		User: shellContainer.User,
		Dir:  shellContainer.WorkingDirectory,

		Privileged: false,
		TTY:        ttySpec,
	}

	result, err := func() (int, error) { // so the term.Restore() can run before the os.Exit()
		var in io.Reader

		if pty.IsTerminal() {
			term, err := pty.OpenRawTerm()
			if err != nil {
				return -1, err
			}

			defer func() {
				_ = term.Restore()
			}()

			in = term
		} else {
			in = os.Stdin
		}

		io := hijacker.ProcessIO{
			In:  in,
			Out: os.Stdout,
			Err: os.Stderr,
		}

		h := hijacker.New(target.TLSConfig(), reqGenerator, target.Token())

		return h.Hijack(target.Team().Name(), shellContainer.ID, spec, io)
	}()

	if err != nil {
		return err
	}

	os.Exit(result)

	return nil
}





