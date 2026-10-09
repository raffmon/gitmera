
package updater

import (
	"fmt"
	"strconv"
	"strings"
)

func CompareVersions(current, latest string) int {
	if current == "dev" || current == "" {
		return -1
	}

	c := parseVersion(current)
	if c == nil {
		return -1
	}

	l := parseVersion(latest)
	if l == nil {
		return 0
	}

	for i := 0; i < 3; i++ {
		if c[i] != l[i] {
			if c[i] < l[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}

	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

func AssetName(version, goos, goarch string) string {
	version = strings.TrimPrefix(version, "v")
	return fmt.Sprintf("gitmera_%s_%s_%s.tar.gz", version, goos, goarch)
}
