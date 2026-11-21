package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
)

type jobsInvocation struct {
	command     string
	environment []string
}

func (invocation jobsInvocation) validateEntrypoint(artifact *compiler.Artifact) error {
	if artifactHasMain(artifact) {
		program := artifact.IR
		discovered := jobs.ManifestFrom(program.Extensions)
		if discovered.HasCommandEntrypoint(program.ModulePath) && jobssql.ManifestFrom(program.Extensions) != nil {
			return nil
		}
	}
	return fmt.Errorf("jobs %s requires an application entrypoint with a configured Jobs adapter and a discovered Job class", invocation.command)
}

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
	return c.runProgramWithJobs(args, jobsInvocation{
		command:     command,
		environment: []string{"TRB_JOBS_COMMAND=" + command, "TRB_JOBS_ID=" + id},
	})
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
	environment := []string{"TRB_JOBS_WORKER=1", "TRB_JOBS_COMMAND="}
	if once {
		environment = append(environment, "TRB_JOBS_ONCE=1")
	}
	if queue != "" {
		environment = append(environment, "TRB_JOBS_QUEUE="+queue)
	}
	return c.runProgramWithJobs(forwarded, jobsInvocation{command: "start", environment: environment})
}

// The worker owns the handler grace period. The CLI allows a bounded final
// interval for claim/database cleanup before forcing the still-owned process
// to exit. Derive both phases from the checked adapter configuration.
func jobsWorkerShutdownTimeout(artifact *compiler.Artifact) time.Duration {
	config := jobssql.DefaultConfig()
	if artifact != nil && artifact.IR != nil {
		if manifest := jobssql.ManifestFrom(artifact.IR.Extensions); manifest != nil {
			config = manifest.Config
		}
	}
	return time.Duration(config.ShutdownTimeoutMilliseconds)*time.Millisecond + 5*time.Second
}
