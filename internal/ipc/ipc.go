package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const SocketName = "wo.sock"

type Request struct {
	Action  string `json:"action"`
	Target  string `json:"target,omitempty"`
	Session string `json:"session,omitempty"`
	Display string `json:"display,omitempty"`
	App     string `json:"app,omitempty"`
}

type Response struct {
	OK       bool          `json:"ok"`
	Error    string        `json:"error,omitempty"`
	Sessions []SessionInfo `json:"sessions,omitempty"`
	Command  string        `json:"command,omitempty"`
}

type SessionInfo struct {
	WaylandDisplay string `json:"wayland_display"`
	Desktop        string `json:"desktop"`
	Seat           string `json:"seat"`
	Active         bool   `json:"active"`
}

func SocketPath() string {
	rd := os.Getenv("XDG_RUNTIME_DIR")
	if rd == "" {
		rd = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(rd, SocketName)
}

func daemonRunning() bool {
	conn, err := net.DialTimeout("unix", SocketPath(), 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func findWodBinary() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(execPath)
	candidate := filepath.Join(dir, "wod")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return exec.LookPath("wod")
}

func EnsureDaemon() error {
	if daemonRunning() {
		return nil
	}

	wod, err := findWodBinary()
	if err != nil {
		return fmt.Errorf("wod binary not found: %w", err)
	}

	cmd := exec.Command(wod)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start wod daemon: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if daemonRunning() {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timed out waiting for wod daemon to start")
}

func SendRequest(req Request) (*Response, error) {
	path := SocketPath()
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("connecting to daemon: %w (is wod running?)", err)
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprintf(conn, "%s\n", data)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &resp, nil
}

func Listen() (net.Listener, error) {
	path := SocketPath()
	os.Remove(path)

	// Ensure runtime dir exists
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0700)

	// Restrict to our UID
	oldMask := syscallUmask(0077)
	defer syscallUmask(oldMask)

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	os.Chmod(path, 0600)
	return l, nil
}

func ReadRequest(conn net.Conn) (*Request, error) {
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = bytesTrimRight(line, '\n')
	if len(line) == 0 {
		return nil, fmt.Errorf("empty request")
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func WriteResponse(conn net.Conn, resp Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(conn, "%s\n", data)
	return err
}
