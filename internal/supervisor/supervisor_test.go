package supervisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

func TestRunOnceRelaysWorkerCompletionAndStreamsStartedFrame(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	workerPath := filepath.Join(dir, "worker.php")
	bootstrapPath := filepath.Join(dir, "bootstrap.php")
	capturedPath := filepath.Join(dir, "captured.json")

	mustWriteExecutable(t, workerPath, `<?php
$input = trim(stream_get_contents(STDIN));
file_put_contents(`+phpString(capturedPath)+`, $input);
$frame = json_decode($input, true, flags: JSON_THROW_ON_ERROR);
echo json_encode([
    'type' => 'task.completed',
    'taskId' => $frame['taskId'],
    'result' => [
        'status' => 'ok',
        'data' => ['message' => 'done'],
        'meta' => [],
        'events' => [[
            'type' => 'task.progress',
            'taskId' => $frame['taskId'],
            'message' => 'worker progress',
            'percent' => 100,
            'meta' => [],
        ]],
    ],
], JSON_THROW_ON_ERROR) . PHP_EOL;
`)
	mustWriteExecutable(t, bootstrapPath, `<?php return null;`)

	input := `{"type":"task.run","taskId":"task-123","name":"fixture.task","taskClass":"Tests\\FixtureTask","payload":{"name":"demo"}}` + "\n"
	cfg := Config{
		PHPBinary:     "php",
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
	t.Parallel()

	dir := t.TempDir()
	workerPath := filepath.Join(dir, "worker.php")
	bootstrapPath := filepath.Join(dir, "bootstrap.php")

	mustWriteExecutable(t, workerPath, `<?php
fwrite(STDERR, "worker exploded\n");
exit(17);
`)
	mustWriteExecutable(t, bootstrapPath, `<?php return null;`)

	cfg := Config{
		PHPBinary:     "php",
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

func phpString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func utf16LEBytes(value string) []byte {
	encoded := utf16.Encode([]rune(value))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, unit := range encoded {
		bytes = append(bytes, byte(unit), byte(unit>>8))
	}

	return bytes
}
