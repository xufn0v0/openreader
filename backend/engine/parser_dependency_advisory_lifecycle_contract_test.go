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
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

const parserDependencyCaseEnv = "OPENREADER_TEST_DEPENDENCY_PARSER_CASE"

func TestParserDependencyAdvisoryBoundedWork(t *testing.T) {
	for _, name := range []string{"html-foreign-close", "html-normal", "norm-invalid-nfc", "norm-invalid-nfkc", "norm-invalid-nfd", "norm-invalid-nfkd", "norm-valid-nfc", "norm-valid-nfkc"} {
		t.Run(name, func(t *testing.T) { runParserDependencyWorker(t, name) })
	}
}

func runParserDependencyWorker(t *testing.T, name string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestParserDependencyAdvisoryWorker$", "-test.count=1")
	command.Env = append(os.Environ(), parserDependencyCaseEnv+"="+name)
	// Reuse the mutex-protected, bounded-output handshake recorder from the
	// XPath tests; these marker names are private test infrastructure only.
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
				t.Error("owned dependency-parser worker was not reaped")
			}
		}
	}()
	checkResult := func(err error) {
		t.Helper()
		text := output.String()
		if !strings.Contains(text, advisoryXPathEntered) {
			t.Fatalf("dependency-parser fixture never entered work: %v %s", err, text)
		}
		if strings.Contains(text, advisoryXPathMemory) {
			t.Fatal("actual dependency-parser work exceeded 32MiB allocation on a tiny input")
		}
		if err != nil || !strings.Contains(text, advisoryXPathCompleted) {
			t.Fatalf("actual dependency-parser work did not complete normally: %v %s", err, text)
		}
	}
	select {
	case <-output.entered:
	case err := <-done:
		waited = true
		checkResult(err)
		return
	case <-time.After(3 * time.Second):
		t.Fatal("dependency-parser fixture startup did not complete; not a behavior red")
	}
	select {
	case err := <-done:
		waited = true
		checkResult(err)
	case <-time.After(3 * time.Second):
		t.Error("actual dependency-parser work did not terminate after entered-work handshake")
	}
}

// Run only in the explicitly owned subprocess. HTML uses the real OpenReader
// document constructor; normalization is deliberately a dependency-level check,
// not a claim that arbitrary invalid bytes reach an exposed service endpoint.
func TestParserDependencyAdvisoryWorker(t *testing.T) {
	name := os.Getenv(parserDependencyCaseEnv)
	if name == "" {
		return
	}
	valid := map[string]bool{"html-foreign-close": true, "html-normal": true,
		"norm-invalid-nfc": true, "norm-invalid-nfkc": true, "norm-invalid-nfd": true,
		"norm-invalid-nfkd": true, "norm-valid-nfc": true, "norm-valid-nfkc": true}
	if !valid[name] {
		t.Fatal("unknown owned dependency-parser fixture")
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
	if strings.HasPrefix(name, "html-") {
		// Minimal input from official Go CL709875 / issue70179. No network.
		input := "<table><tbody><svg><td><desc><select></select></tbody>"
		if name == "html-normal" {
			input = `<article><h1>正常书名</h1><a href="/chapter/one">一段正文</a></article>`
		}
		document, err := newSourceRuleDocument(input)
		if err != nil {
			t.Fatal(err)
		}
		if document == nil || document.document == nil || document.xpathRoot == nil {
			t.Fatal("document constructor omitted a parser representation")
		}
		if name == "html-normal" {
			text, err := sourceRuleString(document.Root(), "@XPath://a/text()")
			if err != nil || text != "一段正文" {
				t.Fatalf("normal HTML text changed: %q %v", text, err)
			}
			link, err := sourceRuleString(document.Root(), "@CSS:a@href")
			if err != nil || link != "/chapter/one" {
				t.Fatalf("normal HTML attribute changed: %q %v", link, err)
			}
		}
	} else {
		form := norm.NFC
		switch name {
		case "norm-invalid-nfkc", "norm-valid-nfkc":
			form = norm.NFKC
		case "norm-invalid-nfd":
			form = norm.NFD
		case "norm-invalid-nfkd":
			form = norm.NFKD
		}
		// Three-byte invalid UTF-8 input from official Go CL794100.
		input, want := "\xf3\xcc\x80", []byte("\xf3\xcc\x80")
		if name == "norm-valid-nfc" {
			input, want = "e\u0301", []byte("é")
		} else if name == "norm-valid-nfkc" {
			input, want = "①", []byte("1")
		}
		var iterator norm.Iter
		iterator.InitString(form, input)
		var result []byte
		for step := 0; !iterator.Done(); step++ {
			if step > len(input)+8 || len(result) > 64 {
				t.Fatalf("normalization iterator did not make bounded progress: steps=%d inputBytes=%d producedBytes=%d position=%d", step, len(input), len(result), iterator.Pos())
			}
			result = append(result, iterator.Next()...)
		}
		if !bytes.Equal(result, want) {
			t.Fatalf("normalization result changed: %x want %x", result, want)
		}
	}
	fmt.Print(advisoryXPathCompleted)
}
