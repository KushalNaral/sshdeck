package importer

import "strings"

// Project is one row of main_screen.
type Project struct {
	Name, OS, PHP, PHPTarget, Server string
}

// Projects reads the main_screen sheet: Project, Type, "OS / PHP / framework", Patched Version, Compat, Server Details.
func Projects(s Sheet) []Project {
	var out []Project
	for i, row := range s.Rows {
		if i == 0 || len(row) == 0 || row[0] == "" {
			continue
		}
		p := Project{Name: row[0], Server: s.Cell(i, 5), PHPTarget: s.Cell(i, 3)}
		p.OS, p.PHP = ParseVersions(s.Cell(i, 2))
		out = append(out, p)
	}
	return out
}

// ParseVersions turns "U 22.04.5/ 8.3.19 / laravel" into ("Ubuntu 22.04.5", "8.3.19"). An unknown PHP ("x") is "".
func ParseVersions(v string) (os, php string) {
	parts := strings.Split(v, "/")
	if len(parts) < 2 {
		return "", ""
	}
	os = strings.TrimSpace(parts[0])
	switch {
	case strings.HasPrefix(os, "U "):
		os = "Ubuntu " + strings.TrimSpace(os[2:])
	case strings.HasPrefix(os, "CentOS"):
		os = "CentOS " + strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(os, "CentOS"), " :"))
	}
	php = strings.TrimSpace(parts[1])
	if len(php) > 0 && php[0] >= '0' && php[0] <= '9' {
		return os, php
	}
	return os, ""
}
