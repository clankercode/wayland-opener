package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/xertrov/wo/internal/session"
)

type Options struct {
	Session      *session.Session
	App          string
	Terminal     bool
	TerminalCmd  string
	DryRun       bool
	SystemdScope bool
}

func Launch(_ string, opts Options) error {
	cmdStr := opts.App
	if cmdStr == "" {
		return fmt.Errorf("no command to launch")
	}

	parts, err := parseCommand(cmdStr)
	if err != nil {
		return err
	}

	if opts.DryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] would execute: %v\n", parts)
		fmt.Fprintf(os.Stderr, "[dry-run] env WAYLAND_DISPLAY=%s\n", opts.Session.WaylandDisplay)
		return nil
	}

	if opts.Terminal && opts.TerminalCmd != "" {
		termParts := strings.Fields(opts.TerminalCmd)
		termParts = append(termParts, "-e")
		termParts = append(termParts, parts...)
		parts = termParts
	}

	if opts.SystemdScope {
		return launchViaSystemd(parts, opts.Session)
	}

	return launchDirect(parts, opts.Session)
}

func parseCommand(cmdStr string) ([]string, error) {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return nil, fmt.Errorf("empty command")
	}

	return strings.Fields(cmdStr), nil
}

func launchDirect(parts []string, sess *session.Session) error {
	cmd := exec.Command(parts[0], parts[1:]...)

	cmd.Env = os.Environ()
	for _, e := range sess.Env() {
		k := strings.SplitN(e, "=", 2)[0]
		cmd.Env = filterEnv(cmd.Env, k)
		cmd.Env = append(cmd.Env, e)
	}

	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", parts[0], err)
	}

	go func() {
		cmd.Wait()
	}()

	return nil
}

func launchViaSystemd(parts []string, sess *session.Session) error {
	args := []string{"--user", "--scope"}
	for _, e := range sess.Env() {
		args = append(args, "--setenv", e)
	}
	args = append(args, "--")
	args = append(args, parts...)

	cmd := exec.Command("systemd-run", args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("systemd-run: %w", err)
	}

	go func() {
		cmd.Wait()
	}()

	return nil
}

func filterEnv(env []string, removeKey string) []string {
	var filtered []string
	for _, e := range env {
		if !strings.HasPrefix(e, removeKey+"=") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
