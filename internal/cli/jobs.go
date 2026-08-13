package cli

import (
	"fmt"
	"os"
	"strings"
)

func (c *CLI) runJobs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("jobs command requires a subcommand")
	}
	switch args[0] {
	case "start":
		return c.runJobsStart(args[1:])
	case "list":
		return c.runJobsCommand("list", "", args[1:])
	case "retry", "discard":
		if len(args) < 2 {
			return fmt.Errorf("jobs %s requires a job ID", args[0])
		}
		return c.runJobsCommand(args[0], args[1], args[2:])
	default:
		return fmt.Errorf("unknown jobs command %q", args[0])
	}
}

func (c *CLI) runJobsCommand(command, id string, args []string) error {
	restoreCommand := temporaryEnvironment("TRB_JOBS_COMMAND", command)
	defer restoreCommand()
	restoreID := temporaryEnvironment("TRB_JOBS_ID", id)
	defer restoreID()
	return c.runProgram(args)
}

func (c *CLI) runJobsStart(args []string) error {
	once := false
	queue := ""
	forwarded := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--once" {
			once = true
			continue
		}
		if argument == "--queue" {
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return fmt.Errorf("jobs start --queue requires a queue name")
			}
			index++
			queue = args[index]
			continue
		}
		if strings.HasPrefix(argument, "--queue=") {
			queue = strings.TrimPrefix(argument, "--queue=")
			if strings.TrimSpace(queue) == "" {
				return fmt.Errorf("jobs start --queue requires a queue name")
			}
			continue
		}
		forwarded = append(forwarded, argument)
	}
	restoreWorker := temporaryEnvironment("TRB_JOBS_WORKER", "1")
	defer restoreWorker()
	if once {
		restoreOnce := temporaryEnvironment("TRB_JOBS_ONCE", "1")
		defer restoreOnce()
	}
	if queue != "" {
		restoreQueue := temporaryEnvironment("TRB_JOBS_QUEUE", queue)
		defer restoreQueue()
	}
	return c.runProgram(forwarded)
}

func temporaryEnvironment(name, value string) func() {
	previous, existed := os.LookupEnv(name)
	_ = os.Setenv(name, value)
	return func() {
		if existed {
			_ = os.Setenv(name, previous)
			return
		}
		_ = os.Unsetenv(name)
	}
}
