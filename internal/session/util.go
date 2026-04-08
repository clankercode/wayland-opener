package session

import (
	"os"
	"strconv"
)

func getUID() uint32 {
	return uint32(os.Getuid())
}

func getRuntimeDir() string {
	if rd := os.Getenv("XDG_RUNTIME_DIR"); rd != "" {
		return rd
	}
	return "/run/user/" + strconv.Itoa(os.Getuid())
}
