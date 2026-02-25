package proxies

import "strings"

func formatBypassHosts(hosts []string) string {
	if len(hosts) == 0 {
		return "-"
	}

	return strings.Join(hosts, ", ")
}
