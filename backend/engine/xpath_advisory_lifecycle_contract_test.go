package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"
)

const advisoryXPathCaseEnv = "OPENREADER_TEST_ADVISORY_XPATH_CASE"
const advisoryXPathEntered = "advisory-xpath-entered\n"
const advisoryXPathCompleted = "advisory-xpath-completed\n"
const advisoryXPathMemory = "advisory-xpath-memory-budget\n"

type advisoryXPathOutput struct {
	mu      sync.Mutex
	data    bytes.Buffer
	entered chan struct{}
	once    sync.Once
}

func (output *advisoryXPathOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n, err := output.data.Write(data)
	if strings.Contains(output.data.String(), advisoryXPathEntered) {
		output.once.Do(func() { close(output.entered) })
	}
	return n, err
}

func (output *advisoryXPathOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.data.String()
}

func TestSourceRuleAdvisoryXPathTerminates(t *testing.T) {
	for _, name := range []string{"true-expression", "true-function", "false-expression", "false-function", "nodes", "text", "attribute", "combined"} {
		t.Run(name, func(t *testing.T) {
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestAdvisoryXPathWorker$", "-test.count=1")
			command.Env = append(os.Environ(), advisoryXPathCaseEnv+"="+name)
			output := &advisoryXPathOutput{entered: make(chan struct{})}
			command.Stdout, command.Stderr = output, output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			done, waited := make(chan error, 1), false
			go func() { done <- command.Wait() }()
			defer func() {
				if !waited {
					cancel()
					_ = command.Process.Kill()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("owned XPath worker was not reaped")
					}
				}
			}()
			select {
			case <-output.entered:
			case err := <-done:
				waited = true
				if !strings.Contains(output.String(), advisoryXPathEntered) {
					t.Fatalf("XPath fixture did not enter evaluator: %v %s", err, output.String())
				}
				if err != nil || !strings.Contains(output.String(), advisoryXPathCompleted) {
					t.Fatalf("actual XPath evaluator failed its bounded worker: %v %s", err, output.String())
				}
				return
			case <-time.After(3 * time.Second):
				t.Fatal("XPath fixture startup did not complete; not a behavior red")
			}
			select {
			case err := <-done:
				waited = true
				if strings.Contains(output.String(), advisoryXPathMemory) {
					t.Fatal("actual XPath evaluator exceeded 32MiB allocation on a one-article document")
				}
				if err != nil || !strings.Contains(output.String(), advisoryXPathCompleted) {
					t.Fatalf("actual XPath evaluator did not finish normally: %v %s", err, output.String())
				}
			case <-time.After(3 * time.Second):
				t.Error("actual XPath evaluator did not terminate after entered-work handshake")
			}
		})
	}
}

// Only the explicitly spawned owned worker executes this body. A watchdog and
// soft memory limit bound the old QueryAll allocation loop; the parent always
// waits or kills+waits this exact process, never leaving an in-process busy loop.
func TestAdvisoryXPathWorker(t *testing.T) {
	name := os.Getenv(advisoryXPathCaseEnv)
	if name == "" {
		return
	}
	fixture, ok := map[string]struct {
		rule string
		want int
	}{
		"true-expression":  {"@XPath:1=1", -1},
		"true-function":    {"@XPath:true()", -1},
		"false-expression": {"@XPath:1=0", 0},
		"false-function":   {"@XPath:false()", 0},
		"nodes":            {"@XPath://article", 1},
		"text":             {"@XPath://article/text()", 1},
		"attribute":        {"@XPath://article/@data-id", 1},
		"combined":         {"@XPath://missing||//article", 1},
	}[name]
	if !ok {
		t.Fatal("unknown owned XPath fixture")
	}
	document, err := newSourceRuleDocument(`<article data-id="owned">safe-owned-text</article>`)
	if err != nil {
		t.Fatal(err)
	}
	debug.SetMemoryLimit(32 << 20)
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	stop := make(chan struct{})
	defer close(stop)
	fmt.Print(advisoryXPathEntered)
	go func() {
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				var current runtime.MemStats
				runtime.ReadMemStats(&current)
				if current.TotalAlloc-before.TotalAlloc > 32<<20 {
					fmt.Fprint(os.Stderr, advisoryXPathMemory)
					os.Exit(86)
				}
			}
		}
	}()
	items, err := sourceRuleElements(document.Root(), fixture.rule)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) > 16 || fixture.want >= 0 && len(items) != fixture.want {
		t.Fatalf("bounded XPath result changed: count=%d want=%d", len(items), fixture.want)
	}
	fmt.Print(advisoryXPathCompleted)
}
