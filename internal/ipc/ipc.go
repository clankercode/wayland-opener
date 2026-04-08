package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
