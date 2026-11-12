//go:build darwin || linux

package cli

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/type-rb/type-rb/internal/project"
)

const shutdownJobSource = `import { Job } from trb/jobs
import { File, FileMode } from trb/std/file
import trb/std/path
import trb/std/result

def released(path: String): Boolean
	result := File.open(Path.new(path)) do |file|
		try file.read_text(max_bytes: 16)
	end
	case result
	when Result::Ok(value)
		return value == "release"
	when Result::Err(_error)
		return false
	end
end

class SampleJob < Job
	def perform(path: String)
		marked := File.open(Path.new(path + ".started"), mode: FileMode::Write) do |file|
			try file.write_text("started")
		end
		case marked
		when Result::Ok(_unit)
		when Result::Err(error)
			puts(error.message)
			return
		end
		while !released(path)
		end
		return
	end
end
`

type jobsShutdownProject struct {
	config   *project.Config
	database string
	release  string
}

func newJobsShutdownProject(t *testing.T, mode string) jobsShutdownProject {
	t.Helper()
	if mode == "ruby" {
		requireRubyORMGems(t, "sequel", "sqlite3")
	}
	if mode == "typescript" {
		if _, err := exec.LookPath("bun"); err != nil {
			t.Skip("bun is not installed")
		}
	}
	root := t.TempDir()
	config := project.New(root, mode)
	config.SourceDir = "src"
	if mode == "go" {
		config.Go.Module = "example.com/jobs-shutdown"
	}
	if mode == "typescript" {
		config.TypeScript.Runtime = project.TypeScriptRuntimeBun
		config.TypeScript.PackageManager = "bun"
	}
	fixture := jobsShutdownProject{config, filepath.Join(root, "jobs.sqlite3"), filepath.Join(root, "release")}
	configureSQLJobs(t, config, "sqlite", fixture.database)
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.SourcePath(), "jobs"), 0o755); err != nil {
		t.Fatal(err)
	}
	main := "import { SampleJob } from jobs/sample_job\ndef main()\nreference := SampleJob.perform_later(" + strconv.Quote(fixture.release) + ") catch |error|\nputs(error.message)\nreturn\nend\nputs(reference.job_name)\nend\n"
	for name, source := range map[string]string{"main.trb": main, "jobs/sample_job.trb": shutdownJobSource} {
		if err := os.WriteFile(filepath.Join(config.SourcePath(), name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return fixture
}

func (f jobsShutdownProject) run(t *testing.T, arguments ...string) {
	t.Helper()
	var output bytes.Buffer
	cli := &CLI{Stdin: strings.NewReader(""), Stdout: &output, Stderr: &output}
	arguments = append(arguments, "--config", f.config.Path)
	if status := cli.Run(arguments); status != 0 {
		t.Fatalf("%v returned %d: %s", arguments, status, output.String())
	}
}

func (f jobsShutdownProject) openDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", f.database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

type shutdownWorker struct {
	command  *exec.Cmd
	wait     chan error
	terminal *os.File
	log      string
	pid      int
	root     string
	finished bool
}

func (f jobsShutdownProject) start(t *testing.T, terminal bool) *shutdownWorker {
	t.Helper()
	command := exec.Command(os.Args[0], "jobs", "start", "--config", f.config.Path)
	command.Env = append(os.Environ(), "TRB_TEST_JOBS_CLI_HELPER=1")
	worker := &shutdownWorker{command: command, wait: make(chan error, 1), log: filepath.Join(t.TempDir(), "worker.log"), root: f.config.Root}
	log, err := os.Create(worker.log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	if terminal {
		worker.terminal, err = pty.Start(command)
		if err != nil {
			t.Fatal(err)
		}
		go func() { _, _ = io.Copy(log, worker.terminal) }()
	} else {
		command.Stdin = strings.NewReader("")
		command.Stdout, command.Stderr = log, log
		configureChildProcess(command, true)
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	go func() { worker.wait <- command.Wait() }()
	t.Cleanup(func() {
		// A failing implementation may return from the CLI before its worker.
		// Check the owned generated path before killing a surviving recorded PID.
		if worker.pid != 0 && syscall.Kill(worker.pid, 0) == nil {
			identity, identityErr := exec.Command("ps", "-p", strconv.Itoa(worker.pid), "-o", "command=").Output()
			if identityErr == nil && strings.Contains(string(identity), filepath.Join(f.config.Root, ".trb", "run")) {
				_ = syscall.Kill(worker.pid, syscall.SIGKILL)
			}
		}
		if !worker.finished {
			_ = forceStopChildProcess(command, !terminal)
			select {
			case <-worker.wait:
			case <-time.After(5 * time.Second):
				t.Error("owned CLI did not stop during test cleanup")
			}
		}
		if worker.terminal != nil {
			_ = worker.terminal.Close()
		}
	})
	return worker
}

func (w *shutdownWorker) signal(t *testing.T, signal os.Signal) {
	t.Helper()
	if w.terminal != nil && signal == os.Interrupt {
		if _, err := w.terminal.Write([]byte{3}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := w.command.Process.Signal(signal); err != nil {
		t.Fatal(err)
	}
}

func (w *shutdownWorker) finish(t *testing.T, timeout time.Duration, success bool) string {
	t.Helper()
	select {
	case err := <-w.wait:
		w.finished = true
		output, _ := os.ReadFile(w.log)
		if (err == nil) != success {
			t.Fatalf("worker CLI error=%v, wanted success=%t: %s", err, success, output)
		}
		if w.pid != 0 && syscall.Kill(w.pid, 0) != syscall.ESRCH {
			t.Fatalf("CLI returned while original worker %d still exists", w.pid)
		}
		return string(output)
	case <-time.After(timeout):
		output, _ := os.ReadFile(w.log)
		// Include queue state when a shutdown stalls, without retaining the
		// generated workspace or a second build cache after the test.
		database, err := sql.Open("sqlite", filepath.Join(w.root, "jobs.sqlite3"))
		if err == nil {
			var state, message string
			_ = database.QueryRow("SELECT state, COALESCE(last_error, '') FROM trb_jobs LIMIT 1").Scan(&state, &message)
			t.Logf("stalled job state=%s error=%s", state, message)
			_ = database.Close()
		}
		t.Fatalf("worker CLI did not finish in %s: %s", timeout, output)
	}
	return ""
}

func (f jobsShutdownProject) waitStarted(t *testing.T, database *sql.DB, worker *shutdownWorker) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var owner string
		if database.QueryRow("SELECT claimed_by FROM trb_jobs WHERE state = 'running'").Scan(&owner) == nil {
			pid, err := strconv.Atoi(strings.TrimPrefix(owner, "worker-"))
			if err != nil {
				t.Fatal(err)
			}
			worker.pid = pid
			if _, err := os.Stat(f.release + ".started"); err == nil {
				return
			}
		}
		select {
		case err := <-worker.wait:
			worker.finished = true
			output, _ := os.ReadFile(worker.log)
			t.Fatalf("CLI exited before job start: %v: %s", err, output)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	output, _ := os.ReadFile(worker.log)
	t.Fatalf("worker did not claim the synthetic job: %s", output)
}

func TestJobsWorkerShutdownAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, terminal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/terminal=%t", mode, terminal), func(t *testing.T) {
				fixture := newJobsShutdownProject(t, mode)
				fixture.run(t, "run")
				database := fixture.openDatabase(t)
				worker := fixture.start(t, terminal)
				fixture.waitStarted(t, database, worker)
				var stop os.Signal = syscall.SIGTERM
				if terminal {
					stop = os.Interrupt
				}
				worker.signal(t, stop)
				select {
				case err := <-worker.wait:
					worker.finished = true
					t.Fatalf("CLI exited before its active handler completed: %v", err)
				case <-time.After(150 * time.Millisecond):
				}
				var claims int
				if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs WHERE state = 'running' AND claimed_by = ?", fmt.Sprintf("worker-%d", worker.pid)).Scan(&claims); err != nil || claims != 1 {
					t.Fatalf("claim released while handler is active: count=%d err=%v", claims, err)
				}
				if err := os.WriteFile(fixture.release, []byte("release"), 0o600); err != nil {
					t.Fatal(err)
				}
				worker.finish(t, 5*time.Second, true)
				if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs").Scan(&claims); err != nil || claims != 0 {
					t.Fatalf("finished handler not acknowledged: count=%d err=%v", claims, err)
				}
				entries, err := os.ReadDir(filepath.Join(fixture.config.Root, ".trb", "run"))
				if err != nil || len(entries) != 0 {
					t.Fatalf("generated workspace still present after shutdown: entries=%v error=%v", entries, err)
				}
			})
		}
	}
}

func TestJobsWorkerForcedShutdownRetainsLease(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newJobsShutdownProject(t, mode)
			fixture.run(t, "run")
			database := fixture.openDatabase(t)
			worker := fixture.start(t, false)
			fixture.waitStarted(t, database, worker)
			worker.signal(t, syscall.SIGTERM)
			time.Sleep(150 * time.Millisecond)
			worker.signal(t, syscall.SIGTERM)
			output := worker.finish(t, 5*time.Second, false)
			if !strings.Contains(output, "forced to stop (repeated stop request)") {
				t.Fatalf("missing forced-stop diagnostic: %s", output)
			}
			var claims int
			if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs WHERE state = 'running' AND claimed_by = ?", fmt.Sprintf("worker-%d", worker.pid)).Scan(&claims); err != nil || claims != 1 {
				t.Fatalf("forced handler's lease must remain for expiry: claims=%d error=%v", claims, err)
			}
		})
	}
}

func TestJobsIdleWorkerStopsAfterAcknowledgement(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newJobsShutdownProject(t, mode)
			fixture.run(t, "run")
			database := fixture.openDatabase(t)
			worker := fixture.start(t, false)
			fixture.waitStarted(t, database, worker)
			if err := os.WriteFile(fixture.release, []byte("release"), 0o600); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				var count int
				if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs").Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("ordinary handler completion was not acknowledged")
				}
				time.Sleep(10 * time.Millisecond)
			}
			worker.signal(t, os.Interrupt)
			worker.finish(t, 5*time.Second, true)
		})
	}
}

func TestGoJobsWorkerGraceExpiryRetainsLeaseUntilRecovery(t *testing.T) {
	fixture := newJobsShutdownProject(t, "go")
	fixture.run(t, "run")
	database := fixture.openDatabase(t)
	worker := fixture.start(t, false)
	fixture.waitStarted(t, database, worker)
	worker.signal(t, syscall.SIGTERM)
	output := worker.finish(t, 40*time.Second, false)
	if !strings.Contains(output, "grace period expired; active claim retained for lease recovery") {
		t.Fatalf("missing grace-expiry diagnostic: %s", output)
	}
	var claims int
	if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs WHERE state = 'running' AND claimed_by = ?", fmt.Sprintf("worker-%d", worker.pid)).Scan(&claims); err != nil || claims != 1 {
		t.Fatalf("grace expiry released an executing claim: claims=%d error=%v", claims, err)
	}
	if _, err := database.Exec("UPDATE trb_jobs SET claimed_at = '2000-01-01 00:00:00' WHERE state = 'running'"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.run(t, "jobs", "start", "--once")
	if err := database.QueryRow("SELECT COUNT(*) FROM trb_jobs").Scan(&claims); err != nil || claims != 0 {
		t.Fatalf("expired claim did not recover and acknowledge: claims=%d error=%v", claims, err)
	}
}
