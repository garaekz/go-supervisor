package supervisor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func TestMain(m *testing.M) {
	switch os.Getenv("GO_SUPERVISOR_TEST_WORKER") {
	case "complete":
		runCompleteTestWorker()
		return
	case "fail":
		fmt.Fprint(os.Stderr, "worker exploded\n")
		os.Exit(17)
	}

	os.Exit(m.Run())
}

func TestRunOnceRelaysWorkerCompletionAndStreamsStartedFrame(t *testing.T) {
	dir := t.TempDir()
	workerPath := filepath.Join(dir, "worker")
	bootstrapPath := filepath.Join(dir, "bootstrap")
	capturedPath := filepath.Join(dir, "captured.json")

	mustWriteExecutable(t, workerPath, ``)
	mustWriteExecutable(t, bootstrapPath, ``)
	t.Setenv("GO_SUPERVISOR_TEST_WORKER", "complete")
	t.Setenv("GO_SUPERVISOR_TEST_CAPTURED_PATH", capturedPath)

	input := `{"type":"task.run","taskId":"task-123","name":"fixture.task","taskClass":"Tests\\FixtureTask","payload":{"name":"demo"}}` + "\n"
	cfg := Config{
		PHPBinary:     os.Args[0],
		WorkerScript:  workerPath,
		BootstrapPath: bootstrapPath,
		WorkingDir:    dir,
	}

	stdout, stderr, code := RunOnce(cfg, input)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", code, stderr)
	}

	frames := decodeFrames(t, stdout)
	if len(frames) != 2 {
		t.Fatalf("expected 2 output frames, got %d: %s", len(frames), stdout)
	}
	if frames[0]["type"] != "task.started" {
		t.Fatalf("expected first frame to be task.started, got %#v", frames[0])
	}
	if frames[0]["taskId"] != "task-123" {
		t.Fatalf("expected started task id to be relayed, got %#v", frames[0]["taskId"])
	}
	if frames[1]["type"] != "task.completed" {
		t.Fatalf("expected second frame to be worker completion, got %#v", frames[1])
	}

	captured, err := os.ReadFile(capturedPath)
	if err != nil {
		t.Fatal(err)
	}
	capturedFrame := map[string]any{}
	if err := json.Unmarshal(captured, &capturedFrame); err != nil {
		t.Fatal(err)
	}
	if capturedFrame["taskClass"] != "Tests\\FixtureTask" {
		t.Fatalf("worker did not receive original task frame: %#v", capturedFrame)
	}
}

func TestRunOnceReturnsFailureFrameWhenWorkerFails(t *testing.T) {
	dir := t.TempDir()
	workerPath := filepath.Join(dir, "worker")
	bootstrapPath := filepath.Join(dir, "bootstrap")

	mustWriteExecutable(t, workerPath, ``)
	mustWriteExecutable(t, bootstrapPath, ``)
	t.Setenv("GO_SUPERVISOR_TEST_WORKER", "fail")

	cfg := Config{
		PHPBinary:     os.Args[0],
		WorkerScript:  workerPath,
		BootstrapPath: bootstrapPath,
		WorkingDir:    dir,
	}

	stdout, stderr, code := RunOnce(cfg, `{"type":"task.run","taskId":"task-fail","name":"fixture.task","taskClass":"Tests\\FixtureTask","payload":[]}`+"\n")
	if code == 0 {
		t.Fatalf("expected non-zero exit code; stdout=%s stderr=%s", stdout, stderr)
	}
	if stderr == "" {
		t.Fatal("expected worker stderr to be surfaced")
	}

	frames := decodeFrames(t, stdout)
	if len(frames) != 2 {
		t.Fatalf("expected started and failed frames, got %d: %s", len(frames), stdout)
	}
	if frames[1]["type"] != "task.failed" {
		t.Fatalf("expected task.failed frame, got %#v", frames[1])
	}
	if frames[1]["taskId"] != "task-fail" {
		t.Fatalf("expected failed task id to be relayed, got %#v", frames[1]["taskId"])
	}
}

func TestNormalizeInputBytesHandlesPowerShellUnicodePipeline(t *testing.T) {
	t.Parallel()

	encoded := append([]byte{0xff, 0xfe}, utf16LEBytes(`{"type":"task.run","taskId":"windows-pipe"}`+"\n")...)

	normalized := NormalizeInputBytes(encoded)
	if normalized != `{"type":"task.run","taskId":"windows-pipe"}`+"\n" {
		t.Fatalf("unexpected normalized input: %q", normalized)
	}
}

func runCompleteTestWorker() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read stdin: %v\n", err)
		os.Exit(2)
	}

	capturedPath := os.Getenv("GO_SUPERVISOR_TEST_CAPTURED_PATH")
	if capturedPath != "" {
		if err := os.WriteFile(capturedPath, input, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write captured input: %v\n", err)
			os.Exit(2)
		}
	}

	frame := map[string]any{}
	if err := json.Unmarshal(input, &frame); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode input: %v\n", err)
		os.Exit(2)
	}

	taskID, _ := frame["taskId"].(string)
	encoded, err := json.Marshal(map[string]any{
		"type":   "task.completed",
		"taskId": taskID,
		"result": map[string]any{
			"status": "ok",
			"data": map[string]any{
				"message": "done",
			},
			"meta": []any{},
			"events": []map[string]any{
				{
					"type":    "task.progress",
					"taskId":  taskID,
					"message": "worker progress",
					"percent": 100,
					"meta":    []any{},
				},
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode output: %v\n", err)
		os.Exit(2)
	}

	fmt.Println(string(encoded))
}

func decodeFrames(t *testing.T, output string) []map[string]any {
	t.Helper()

	lines := splitNonEmptyLines(output)
	frames := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("invalid json frame %q: %v", line, err)
		}
		frames = append(frames, frame)
	}

	return frames
}

func mustWriteExecutable(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func utf16LEBytes(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, unit := range encoded {
		bytes = append(bytes, byte(unit), byte(unit>>8))
	}

	return bytes
}
