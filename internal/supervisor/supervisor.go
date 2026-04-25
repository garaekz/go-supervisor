package supervisor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"
)

type Config struct {
	PHPBinary     string
	WorkerScript  string
	BootstrapPath string
	WorkingDir    string
}

func NormalizeInputBytes(input []byte) string {
	if len(input) >= 2 && input[0] == 0xff && input[1] == 0xfe {
		return string(decodeUTF16LE(input[2:]))
	}

	if len(input) >= 3 && input[0] == 0xef && input[1] == 0xbb && input[2] == 0xbf {
		return string(input[3:])
	}

	return string(input)
}

func RunOnce(cfg Config, input string) (stdout string, stderr string, exitCode int) {
	frame, err := decodeFrame(input)
	if err != nil {
		failed := failureFrame("task-unknown", "invalid task frame: "+err.Error())
		return encodeFrame(failed), err.Error(), 2
	}

	taskID := stringValue(frame, "taskId")
	if taskID == "" {
		taskID = "task-unknown"
	}

	var out bytes.Buffer
	writeFrame(&out, map[string]any{
		"type":   "task.started",
		"taskId": taskID,
		"name":   frame["name"],
	})

	phpBinary := cfg.PHPBinary
	if phpBinary == "" {
		phpBinary = "php"
	}

	cmd := exec.Command(phpBinary, cfg.WorkerScript, "--bootstrap="+cfg.BootstrapPath, "--once")
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}
	cmd.Stdin = strings.NewReader(input)

	var workerStdout bytes.Buffer
	var workerStderr bytes.Buffer
	cmd.Stdout = &workerStdout
	cmd.Stderr = &workerStderr

	if err := cmd.Run(); err != nil {
		writeFrame(&out, failureFrame(taskID, workerStderr.String()))
		return out.String(), workerStderr.String(), processExitCode(err)
	}

	out.Write(workerStdout.Bytes())

	return out.String(), workerStderr.String(), 0
}

func decodeUTF16LE(input []byte) []rune {
	if len(input)%2 != 0 {
		input = input[:len(input)-1]
	}

	units := make([]uint16, 0, len(input)/2)
	for index := 0; index < len(input); index += 2 {
		units = append(units, uint16(input[index])|uint16(input[index+1])<<8)
	}

	return utf16.Decode(units)
}

func decodeFrame(input string) (map[string]any, error) {
	lines := splitNonEmptyLines(input)
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	frame := map[string]any{}
	if err := json.Unmarshal([]byte(lines[0]), &frame); err != nil {
		return nil, err
	}

	return frame, nil
}

func splitNonEmptyLines(input string) []string {
	rawLines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

func failureFrame(taskID string, message string) map[string]any {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "worker exited unsuccessfully"
	}

	return map[string]any{
		"type":    "task.failed",
		"taskId":  taskID,
		"message": message,
	}
}

func writeFrame(out *bytes.Buffer, frame map[string]any) {
	out.WriteString(encodeFrame(frame))
}

func encodeFrame(frame map[string]any) string {
	encoded, err := json.Marshal(frame)
	if err != nil {
		panic(err)
	}

	return string(encoded) + "\n"
}

func stringValue(frame map[string]any, key string) string {
	value, ok := frame[key].(string)
	if !ok {
		return ""
	}

	return value
}

func processExitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}

	return 1
}
