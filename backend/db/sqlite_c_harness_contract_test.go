package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

const sqliteHarnessProbe = "OPENREADER_TEST_SQLITE_HARNESS_PROBE"

// Invoked only in an owned subprocess. Its failure is the expected observation
// of the harness rejecting a non-SQL fault, not a SQLite correctness red.
func TestSQLiteCMaintenanceHarnessProbe(t *testing.T) {
	name := os.Getenv(sqliteHarnessProbe)
	if name == "" {
		return
	}
	if name != "output" && name != "deadline" && name != "sigint" && name != "sigterm" {
		t.Fatal("unknown harness probe")
	}
	runSQLiteMaintenanceWorker(t, "harness-"+name)
}

func TestSQLiteCMaintenanceHarnessFaults(t *testing.T) {
	for _, name := range []string{"output", "deadline", "sigint", "sigterm"} {
		t.Run(name, func(t *testing.T) {
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSQLiteCMaintenanceHarnessProbe$", "-test.count=1", "-test.v=true")
			cmd.Env = append(os.Environ(), sqliteHarnessProbe+"="+name)
			// Observation failures must not orphan a grandchild. A new process
			// group contains only this owned parent and its worker, not the root.
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
			cmd.WaitDelay = 2 * time.Second
			output := &sqliteMaintenanceOutput{entered: make(chan struct{}), cancel: cancel}
			cmd.Stdout, cmd.Stderr = output, output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			var waitErr error
			if name == "sigint" || name == "sigterm" {
				select {
				case <-output.entered:
					sig := syscall.SIGINT
					if name == "sigterm" {
						sig = syscall.SIGTERM
					}
					// This PID came from our own Start, never a process search.
					if err := cmd.Process.Signal(sig); err != nil {
						cancel()
						<-done
						t.Fatal(err)
					}
				case waitErr = <-done:
					t.Fatalf("owned parent exited before interrupt readiness: %v", waitErr)
				case <-ctx.Done():
					<-done
					t.Fatal("harness observation timeout, not a successful fault check")
				}
			}
			waitErr = <-done
			text, overflow := output.snapshot()
			if ctx.Err() != nil || overflow {
				t.Fatalf("outer harness failed: context=%v overflow=%v", ctx.Err(), overflow)
			}
			var exitErr *exec.ExitError
			if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("owned parent must reject fixture through testing, got %v: %s", waitErr, text)
			}
			want := "SQLite fixture interrupted"
			if name == "output" {
				want = "owned SQLite worker exceeded output budget"
			} else if name == "deadline" {
				want = "SQLite work deadline (not a behavior red)"
			}
			if !strings.Contains(text, want) || strings.Contains(text, "owned SQLite worker not reaped") {
				t.Fatalf("wrong rejection or missing reap: %s", text)
			}
			pid := 0
			for _, line := range strings.Split(text, "\n") {
				if strings.HasPrefix(line, "SQLITE_HARNESS_WORKER_PID=") {
					if _, err := fmt.Sscanf(line, "SQLITE_HARNESS_WORKER_PID=%d", &pid); err != nil {
						t.Fatal(err)
					}
				}
			}
			if pid <= 0 {
				t.Fatalf("missing owned worker PID: %s", text)
			}
			// Signal zero only observes the recorded owned PID; it sends no signal.
			if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("owned worker %d still present after parent Wait: %v", pid, err)
			}
		})
	}
}
