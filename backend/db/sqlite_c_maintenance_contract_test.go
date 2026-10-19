package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
	"openreader/backend/config"
)

const sqliteMaintenanceCase = "OPENREADER_TEST_SQLITE_MAINTENANCE_CASE"
const sqliteEntered = "SQLITE_MAINTENANCE_ENTERED\n"
const sqliteCompleted = "SQLITE_MAINTENANCE_COMPLETED\n"

// These are library-level, two-row/empty-table correctness checks, not an API
// exploit or a reproducer for the separate WAL-reset critical timing window.
func TestSQLiteCMaintenanceBoundedBehavior(t *testing.T) {
	for _, name := range []string{"empty-aggregate-not-null", "redundant-upsert-index", "bound-values-simple-upsert", "null-empty-control", "wal-rollback-checkpoint-reopen", "prepared-bindings-reuse", "early-rows-close-reuse", "query-transaction-cancel-reuse", "in-flight-query-cancel", "in-flight-exec-cancel", "runtime-features"} {
		t.Run(name, func(t *testing.T) { runSQLiteMaintenanceWorker(t, name) })
	}
}

type sqliteMaintenanceOutput struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	entered  chan struct{}
	once     sync.Once
	cancel   context.CancelFunc
	tooLarge bool
}

func (o *sqliteMaintenanceOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	if o.buf.Len()+len(p) > 16<<10 {
		o.tooLarge = true
		o.mu.Unlock()
		o.cancel()
		return len(p), nil
	}
	_, _ = o.buf.Write(p)
	if bytes.Contains(o.buf.Bytes(), []byte(sqliteEntered)) {
		o.once.Do(func() { close(o.entered) })
	}
	o.mu.Unlock()
	return len(p), nil
}

func (o *sqliteMaintenanceOutput) snapshot() (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String(), o.tooLarge
}

func runSQLiteMaintenanceWorker(t *testing.T, name string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSQLiteCMaintenanceOwnedWorker$", "-test.count=1")
	cmd.Env = append(os.Environ(), sqliteMaintenanceCase+"="+name)
	cmd.WaitDelay = 2 * time.Second
	output := &sqliteMaintenanceOutput{entered: make(chan struct{}), cancel: cancel}
	cmd.Stdout, cmd.Stderr = output, output
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(name, "harness-") {
		fmt.Printf("SQLITE_HARNESS_WORKER_PID=%d\n", cmd.Process.Pid)
	}
	done, waited := make(chan error, 1), false
	go func() { done <- cmd.Wait() }()
	defer func() {
		if !waited {
			cancel()
			_ = cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("owned SQLite worker not reaped")
			}
		}
	}()
	check := func(err error) {
		t.Helper()
		text, tooLarge := output.snapshot()
		if tooLarge {
			t.Fatal("owned SQLite worker exceeded output budget")
		}
		if !strings.Contains(text, sqliteEntered) {
			t.Fatalf("SQLite fixture never entered work (not a behavior red): %v %s", err, text)
		}
		if err != nil || !strings.Contains(text, sqliteCompleted) {
			t.Fatalf("actual SQLite invariant failed: %v %s", err, text)
		}
		if name == "runtime-features" || strings.HasPrefix(name, "in-flight-") {
			t.Logf("owned runtime query: %s", text)
		}
	}
	select {
	case <-output.entered:
	case err := <-done:
		waited = true
		check(err)
		return
	case <-interrupt:
		t.Fatal("SQLite fixture interrupted")
	case <-time.After(5 * time.Second):
		t.Fatal("SQLite startup deadline (not a behavior red)")
	}
	if strings.HasPrefix(name, "harness-") {
		// Tell the outer, owned harness test that its grandchild is in work and
		// this parent has installed its interrupt handler. Never signal the root.
		fmt.Print(sqliteEntered)
	}
	select {
	case err := <-done:
		waited = true
		check(err)
	case <-interrupt:
		t.Fatal("SQLite fixture interrupted")
	case <-time.After(2 * time.Second):
		t.Fatal("SQLite work deadline (not a behavior red)")
	}
}

func TestSQLiteCMaintenanceOwnedWorker(t *testing.T) {
	name := os.Getenv(sqliteMaintenanceCase)
	if name == "" {
		return
	}
	valid := map[string]bool{"empty-aggregate-not-null": true, "redundant-upsert-index": true,
		"bound-values-simple-upsert": true, "null-empty-control": true, "wal-rollback-checkpoint-reopen": true,
		"prepared-bindings-reuse": true, "early-rows-close-reuse": true, "query-transaction-cancel-reuse": true,
		"in-flight-query-cancel": true, "in-flight-exec-cancel": true, "runtime-features": true,
		"harness-output": true, "harness-deadline": true, "harness-sigint": true, "harness-sigterm": true}
	if !valid[name] {
		t.Fatal("unknown owned SQLite case")
	}
	version, _, _ := sqlite3.Version()
	t.Logf("actual C SQLite version %s; bounded owned fixture %s", version, name)
	fmt.Print(sqliteEntered)
	if strings.HasPrefix(name, "harness-") {
		if name == "harness-output" {
			fmt.Print(strings.Repeat("x", 17<<10))
		}
		// A fixed idle fixture, never SQL/CPU stress. The real parent must kill
		// and Wait it on output overflow, work timeout, SIGINT, or SIGTERM.
		time.Sleep(30 * time.Second)
		t.Fatal("harness fixture was not stopped by its owned parent")
	}
	if name == "wal-rollback-checkpoint-reopen" {
		runSQLiteWALNormalControl(t)
		fmt.Print(sqliteCompleted)
		return
	}
	if name == "query-transaction-cancel-reuse" {
		runSQLiteCancellationControl(t)
		fmt.Print(sqliteCompleted)
		return
	}
	if name == "in-flight-query-cancel" || name == "in-flight-exec-cancel" {
		runSQLiteInFlightCancellationControl(t, name == "in-flight-exec-cancel")
		fmt.Print(sqliteCompleted)
		return
	}
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	execute := func(statement string, args ...any) {
		t.Helper()
		if _, err := database.Exec(statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	switch name {
	case "runtime-features":
		var version, sourceID string
		if err := database.QueryRow("SELECT sqlite_version(),sqlite_source_id()").Scan(&version, &sourceID); err != nil {
			t.Fatal(err)
		}
		bindingVersion, _, bindingSource := sqlite3.Version()
		if version != bindingVersion || sourceID != bindingSource || sourceID == "" {
			t.Fatalf("SQL and linked C identity disagree: SQL=%s/%s binding=%s/%s", version, sourceID, bindingVersion, bindingSource)
		}
		rows, err := database.Query("PRAGMA compile_options")
		if err != nil {
			t.Fatal(err)
		}
		options := make(map[string]bool)
		for rows.Next() {
			var option string
			if err := rows.Scan(&option); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			options[option] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"ENABLE_FTS3", "ENABLE_FTS3_PARENTHESIS", "ENABLE_RTREE", "THREADSAFE=1"} {
			if !options[required] {
				t.Fatalf("missing existing runtime capability %s", required)
			}
		}
		if options["ENABLE_FTS5"] {
			t.Fatal("unexpected FTS5 activation changes the evaluated C feature surface")
		}
		// Only self-owned version/capability metadata, never rows/user content.
		fmt.Printf("SQLITE_RUNTIME version=%s source=%s options=%v\n", version, sourceID, options)
	case "empty-aggregate-not-null":
		execute("CREATE TABLE empty_probe (value INTEGER NOT NULL)")
		var isNull, count int
		if err := database.QueryRow("SELECT value IS NULL, count(*) FROM empty_probe").Scan(&isNull, &count); err != nil {
			t.Fatal(err)
		}
		if isNull != 1 || count != 0 {
			t.Fatalf("empty aggregate bare value must be NULL: got (%d,%d), want (1,0)", isNull, count)
		}
	case "redundant-upsert-index":
		execute("CREATE TABLE key_probe (id INTEGER PRIMARY KEY ON CONFLICT REPLACE, value INTEGER UNIQUE)")
		execute("INSERT INTO key_probe VALUES (0,33),(11,22)")
		// Own two-row variant of the maintainer's redundant conflict-clause boundary.
		execute("REPLACE INTO key_probe VALUES (0,11) ON CONFLICT(value) DO UPDATE SET id=value,value=value ON CONFLICT(value) DO UPDATE SET id=id,value=id")
		var tableCount, indexCount int
		if err := database.QueryRow("SELECT count(*) FROM key_probe NOT INDEXED").Scan(&tableCount); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRow("SELECT count(*) FROM key_probe WHERE value>8").Scan(&indexCount); err != nil {
			t.Fatal(err)
		}
		if tableCount != 2 || indexCount != tableCount {
			t.Fatalf("index disagrees with table: table=%d index=%d", tableCount, indexCount)
		}
		var integrity string
		if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
			t.Fatalf("owned index integrity=%q err=%v", integrity, err)
		}
	case "bound-values-simple-upsert":
		execute("CREATE TABLE normal_probe (id INTEGER PRIMARY KEY, value TEXT UNIQUE)")
		value := "中文 ' ; -- 一段正常的绑定数据"
		execute("INSERT INTO normal_probe VALUES (?,?)", 1, value)
		execute("INSERT INTO normal_probe VALUES (?,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value", 1, value+"更新")
		var actual string
		if err := database.QueryRow("SELECT value FROM normal_probe WHERE id=?", 1).Scan(&actual); err != nil || actual != value+"更新" {
			t.Fatalf("bound upsert changed: %q %v", actual, err)
		}
	case "null-empty-control":
		execute("CREATE TABLE null_probe (value INTEGER)")
		var value sql.NullInt64
		var count int
		if err := database.QueryRow("SELECT value,count(*) FROM null_probe").Scan(&value, &count); err != nil || value.Valid || count != 0 {
			t.Fatalf("normal empty aggregate: value=%v count=%d err=%v", value, count, err)
		}
	case "prepared-bindings-reuse":
		statement, err := database.Prepare("SELECT ?, ?")
		if err != nil {
			t.Fatal(err)
		}
		defer statement.Close()
		for _, value := range []any{"中文正文" + strings.Repeat("长", 64), "短", nil, "", "恢复"} {
			var actual sql.NullString
			var marker int
			if err := statement.QueryRow(value, 7).Scan(&actual, &marker); err != nil {
				t.Fatal(err)
			}
			if marker != 7 || actual.Valid != (value != nil) || (value != nil && actual.String != value.(string)) {
				t.Fatalf("prepared binding retained prior value: value=%v actual=%v marker=%d", value, actual, marker)
			}
		}
	case "early-rows-close-reuse":
		statement, err := database.Prepare("SELECT ? UNION ALL SELECT ?")
		if err != nil {
			t.Fatal(err)
		}
		defer statement.Close()
		for i := 0; i < 16; i++ {
			rows, err := statement.Query(i, i+1)
			if err != nil {
				t.Fatal(err)
			}
			if !rows.Next() {
				rows.Close()
				t.Fatalf("missing first row: %v", rows.Err())
			}
			var actual int
			if err := rows.Scan(&actual); err != nil || actual != i {
				rows.Close()
				t.Fatalf("prepared row=%d want=%d err=%v", actual, i, err)
			}
			// Leave the second row unread. Both Close calls must release/reset the
			// statement so the single connection can be reused without stale rows.
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			if err := database.QueryRow("SELECT ?", i+100).Scan(&actual); err != nil || actual != i+100 {
				t.Fatalf("connection unavailable after early close: %d %v", actual, err)
			}
		}
	}
	fmt.Print(sqliteCompleted)
}

// Normal lifecycle control, not a claim to cover interruption during an active C
// step. The file is owned by this worker: cancellation may discard a connection,
// and a fresh :memory: connection would otherwise hide rollback/reopen behavior.
func runSQLiteCancellationControl(t *testing.T) {
	t.Helper()
	database, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "cancel-control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	if _, err := database.Exec("CREATE TABLE cancel_probe(id INTEGER PRIMARY KEY,value TEXT); INSERT INTO cancel_probe VALUES(1,'原值'),(2,'第二行')"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rows, err := database.QueryContext(ctx, "SELECT value FROM cancel_probe ORDER BY id")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if !rows.Next() {
		cancel()
		rows.Close()
		t.Fatalf("missing cancellation control row: %v", rows.Err())
	}
	var first string
	if err := rows.Scan(&first); err != nil || first != "原值" {
		cancel()
		rows.Close()
		t.Fatalf("first row=%q err=%v", first, err)
	}
	cancel()
	if rows.Next() || !errors.Is(rows.Err(), context.Canceled) {
		rows.Close()
		t.Fatalf("canceled rows did not stop: %v", rows.Err())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE cancel_probe SET value=? WHERE id=1", "取消的更改"); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if err := tx.Commit(); !errors.Is(err, context.Canceled) && !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("canceled transaction unexpectedly committed: %v", err)
	}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer checkCancel()
	var actual, integrity string
	if err := database.QueryRowContext(checkCtx, "SELECT value FROM cancel_probe WHERE id=1").Scan(&actual); err != nil || actual != "原值" {
		t.Fatalf("cancel did not preserve committed data/release pool: value=%q err=%v", actual, err)
	}
	if err := database.QueryRowContext(checkCtx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("cancel control integrity=%q err=%v", integrity, err)
	}
}

// In-flight control: the context must interrupt a statement while its C step is
// still running, not only between rows. The connection-local marker proves the
// VDBE program began; the one-row recursive count keeps memory constant and would
// otherwise run far past the owned worker deadline. While the step may still be in
// C, failures exit directly because deferred pool/transaction cleanup would wait
// on it and turn a behavior failure into a harness deadline.
func runSQLiteInFlightCancellationControl(t *testing.T, write bool) {
	t.Helper()
	database, err := Open(config.Config{DatabasePath: filepath.Join(t.TempDir(), "in-flight-cancel.db")})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	if _, err := pool.Exec("CREATE TABLE in_flight_probe(id INTEGER PRIMARY KEY,value TEXT); INSERT INTO in_flight_probe VALUES(1,'原值'),(2,'第二行')"); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	entered := make(chan struct{})
	var once sync.Once
	if err := conn.Raw(func(driverConn any) error {
		sqliteConn, ok := driverConn.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", driverConn)
		}
		return sqliteConn.RegisterFunc("in_flight_marker", func() int64 {
			once.Do(func() { close(entered) })
			return 1
		}, false)
	}); err != nil {
		t.Fatal(err)
	}
	abandon := func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
		os.Exit(1)
	}
	const spin = "WITH RECURSIVE spin(x) AS (SELECT in_flight_marker() UNION ALL SELECT x+1 FROM spin WHERE x<?) "
	const limit = int64(4_000_000_000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	var count int64
	var tx *sql.Tx
	if write {
		if tx, err = conn.BeginTx(ctx, nil); err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, "UPDATE in_flight_probe SET value=? WHERE id=1", "取消的更改"); err != nil {
			t.Fatal(err)
		}
		go func() {
			_, err := tx.ExecContext(ctx, spin+"INSERT INTO in_flight_probe(id,value) SELECT 3,count(*) FROM spin", limit)
			done <- err
		}()
	} else {
		go func() { done <- conn.QueryRowContext(ctx, spin+"SELECT count(*) FROM spin", limit).Scan(&count) }()
	}
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("statement returned before its C step began (precondition, not a behavior red): %v", err)
	case <-time.After(time.Second):
		abandon("in-flight marker not reached (precondition, not a behavior red)")
	}
	time.Sleep(50 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("statement finished before cancellation (precondition, not a behavior red): %v", err)
	default:
	}
	canceledAt := time.Now()
	cancel()
	var latency time.Duration
	select {
	case err := <-done:
		latency = time.Since(canceledAt)
		if !errors.Is(err, context.Canceled) || count != 0 {
			t.Fatalf("in-flight cancel returned err=%v count=%d after %v", err, count, latency)
		}
	case <-time.After(time.Second):
		abandon("in-flight C step ignored cancellation for 1s")
	}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer checkCancel()
	if write {
		if err := tx.Commit(); !errors.Is(err, context.Canceled) && !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("interrupted transaction unexpectedly committed: %v", err)
		}
	} else {
		// The interrupted statement must be reset so the same C connection serves
		// the next query instead of keeping the interrupt or a busy statement.
		var marker int
		if err := conn.QueryRowContext(checkCtx, "SELECT ?", 42).Scan(&marker); err != nil || marker != 42 {
			t.Fatalf("connection unusable after in-flight cancel: %d %v", marker, err)
		}
	}
	_ = conn.Close()
	var value, integrity string
	var rows int
	if err := pool.QueryRowContext(checkCtx, "SELECT value FROM in_flight_probe WHERE id=1").Scan(&value); err != nil || value != "原值" {
		t.Fatalf("in-flight cancel did not preserve committed data/release pool: value=%q err=%v", value, err)
	}
	if err := pool.QueryRowContext(checkCtx, "SELECT count(*) FROM in_flight_probe").Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("interrupted statement left rows=%d err=%v", rows, err)
	}
	if err := pool.QueryRowContext(checkCtx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("in-flight cancel integrity=%q err=%v", integrity, err)
	}
	mode := "query"
	if write {
		mode = "exec-transaction"
	}
	fmt.Printf("SQLITE_IN_FLIGHT_CANCEL mode=%s latency=%s\n", mode, latency.Round(time.Microsecond))
}

func runSQLiteWALNormalControl(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "owned-wal.db")
	database, err := Open(config.Config{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	pool.SetMaxOpenConns(3)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, err := pool.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pool.Conn(ctx)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	defer first.Close()
	defer second.Close()
	for _, conn := range []*sql.Conn{first, second} {
		var mode string
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
			t.Fatalf("WAL mode=%q err=%v", mode, err)
		}
	}
	for _, statement := range []string{"CREATE TABLE normal_wal(id INTEGER PRIMARY KEY,value TEXT)", "INSERT INTO normal_wal VALUES(1,'原有正文')"} {
		if _, err := first.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := first.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE normal_wal SET value=? WHERE id=1", "不应提交"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var busy, log, checkpointed int
	if err := second.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed); err != nil || busy != 0 {
		t.Fatalf("checkpoint=%d/%d/%d err=%v", busy, log, checkpointed, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var actual, integrity string
	if err := reopened.QueryRow("SELECT value FROM normal_wal WHERE id=1").Scan(&actual); err != nil || actual != "原有正文" {
		t.Fatalf("rollback/reopen lost original: %q %v", actual, err)
	}
	if err := reopened.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("normal WAL integrity=%q err=%v", integrity, err)
	}
}
