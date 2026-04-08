package handler

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Handler struct {
	Exec     string
	Name     string
	Terminal bool
}

func FindHandler(target string) (*Handler, error) {
	mime := detectMIME(target)
	if mime == "" {
		return nil, fmt.Errorf("could not detect MIME type for %q", target)
	}

	h, err := findDesktopFileHandler(mime)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func detectMIME(target string) string {
	if isURL(target) {
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return "x-scheme-handler/https"
		}
		if strings.HasPrefix(target, "mailto:") {
			return "x-scheme-handler/mailto"
		}
		schemeEnd := strings.Index(target, ":")
		if schemeEnd > 0 {
			return "x-scheme-handler/" + target[:schemeEnd]
		}
	}

	cmd := exec.Command("file", "--mime-type", "--brief", target)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isURL(target string) bool {
	for _, prefix := range []string{"http://", "https://", "ftp://", "mailto:", "magnet:", "tel:", "steam://"} {
		if strings.HasPrefix(target, prefix) {
			return true
		}
	}
	return false
}

func findDesktopFileHandler(mime string) (*Handler, error) {
	defaultApp := getDefaultApp(mime)
	if defaultApp == "" {
		return nil, fmt.Errorf("no handler for MIME type %q", mime)
	}

	return parseDesktopFile(defaultApp)
}

func getDefaultApp(mime string) string {
	if app := getMimeappsListDefault(mime); app != "" {
		return app
	}
	if app := queryGioDefault(mime); app != "" {
		return app
	}
	return ""
}

func getMimeappsListDefault(mime string) string {
	paths := []string{
		filepath.Join(os.Getenv("HOME"), ".config/mimeapps.list"),
		"/usr/share/applications/mimeapps.list",
		"/usr/local/share/applications/mimeapps.list",
		filepath.Join(os.Getenv("HOME"), ".local/share/applications/mimeapps.list"),
	}

	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()

		inDefaultApps := false
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "[Default Applications]" {
				inDefaultApps = true
				continue
			}
			if strings.HasPrefix(line, "[") {
				inDefaultApps = false
				continue
			}
			if inDefaultApps && strings.HasPrefix(line, mime+"=") {
				app := strings.TrimPrefix(line, mime+"=")
				app = strings.Split(app, ";")[0]
				return resolveDesktopFile(app)
			}
		}
	}
	return ""
}

func queryGioDefault(mime string) string {
	cmd := exec.Command("gio", "mime", mime)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".desktop") {
			return resolveDesktopFile(filepath.Base(line))
		}
	}
	return ""
}

func resolveDesktopFile(name string) string {
	if strings.Contains(name, "/") {
		return name
	}
	if !strings.HasSuffix(name, ".desktop") {
		name += ".desktop"
	}

	searchDirs := []string{
		filepath.Join(os.Getenv("HOME"), ".local/share/applications"),
		"/usr/local/share/applications",
		"/usr/share/applications",
		"/var/lib/flatpak/exports/share/applications",
		filepath.Join(os.Getenv("HOME"), ".local/share/flatpak/exports/share/applications"),
	}

	for _, dir := range searchDirs {
		full := filepath.Join(dir, name)
		if _, err := os.Stat(full); err == nil {
			return full
		}
	}
	return name
}

func parseDesktopFile(path string) (*Handler, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening desktop file %s: %w", path, err)
	}
	defer f.Close()

	h := &Handler{
		Name: filepath.Base(path),
	}

	scanner := bufio.NewScanner(f)
	inDesktopEntry := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[Desktop Entry]" {
			inDesktopEntry = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			if inDesktopEntry {
				break
			}
			continue
		}
		if !inDesktopEntry {
			continue
		}
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "Exec":
			h.Exec = expandExec(val)
		case "Name":
			h.Name = val
		case "Terminal":
			h.Terminal = val == "true"
		}
	}

	if h.Exec == "" {
		return nil, fmt.Errorf("no Exec line in %s", path)
	}
	return h, nil
}

func expandExec(execLine string) string {
	// Strip field codes: %f, %F, %u, %U, %d, %D, %n, %N, %i, %c, %k, %v, %m
	replacements := []string{
		"%f", "", "%F", "",
		"%u", "", "%U", "",
		"%d", "", "%D", "",
		"%n", "", "%N", "",
		"%i", "", "%c", "",
		"%k", "", "%v", "",
		"%m", "",
	}
	result := execLine
	for i := 0; i < len(replacements); i += 2 {
		result = strings.ReplaceAll(result, replacements[i], replacements[i+1])
	}
	return strings.TrimSpace(result)
}

func PrepareCommand(execLine string, target string) string {
	// If exec line already has the target placeholder expanded (empty), append target
	cmd := strings.TrimSpace(execLine)
	if cmd == "" {
		return target
	}
	// If there was a field code, the space is already there
	if strings.HasSuffix(cmd, " ") {
		return cmd + quoteArg(target)
	}
	return cmd + " " + quoteArg(target)
}

func quoteArg(arg string) string {
	if !strings.ContainsAny(arg, " \t\"'\\$`!#&|;(){}<>?*[]") {
		return arg
	}
	buf := new(bytes.Buffer)
	buf.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '"', '\\', '$', '`':
			buf.WriteByte('\\')
			buf.WriteRune(r)
		default:
			buf.WriteRune(r)
		}
	}
	buf.WriteByte('"')
	return buf.String()
}
