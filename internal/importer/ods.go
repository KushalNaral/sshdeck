// Package importer reads the master project list (.ods) so servers and VPN profiles can be seeded.
package importer

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// Sheet is one tab. Rows with no text are dropped; cell positions within a row are kept.
type Sheet struct {
	Name string
	Rows [][]string
}

type odsCell struct {
	Rep int `xml:"number-columns-repeated,attr"`
	P   []struct {
		Inner string `xml:",innerxml"`
	} `xml:"p"`
}
type odsRow struct {
	Cells []odsCell `xml:",any"` // table-cell and covered-table-cell, in order
}
type odsTable struct {
	Name string   `xml:"name,attr"`
	Rows []odsRow `xml:"table-row"`
}
type odsDoc struct {
	Tables []odsTable `xml:"body>spreadsheet>table"`
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func (c odsCell) text() string {
	var parts []string
	for _, p := range c.P {
		parts = append(parts, html.UnescapeString(tagRe.ReplaceAllString(p.Inner, "")))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// ReadODS returns every sheet in the file.
func ReadODS(path string) ([]Sheet, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "content.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}
		var d odsDoc
		if err := xml.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("read content.xml: %w", err)
		}
		var out []Sheet
		for _, t := range d.Tables {
			s := Sheet{Name: t.Name}
			for _, r := range t.Rows {
				var cells []string
				for _, c := range r.Cells {
					n := max(c.Rep, 1)
					if n > 30 { // trailing "repeat to the edge" padding
						n = 1
					}
					for range n {
						cells = append(cells, c.text())
					}
				}
				for len(cells) > 0 && cells[len(cells)-1] == "" {
					cells = cells[:len(cells)-1]
				}
				if len(cells) > 0 {
					s.Rows = append(s.Rows, cells)
				}
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s: no content.xml, not an .ods file", path)
}

// Cell returns the text at row r, column c, or "" when it does not exist.
func (s Sheet) Cell(r, c int) string {
	if r < 0 || r >= len(s.Rows) || c < 0 || c >= len(s.Rows[r]) {
		return ""
	}
	return s.Rows[r][c]
}

// Value returns what follows a "Label: value" cell, or the cell itself when it has no label.
func Value(cell string) string {
	if _, v, ok := strings.Cut(cell, ":"); ok {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(cell)
}

// Pairs reads label/value cells side by side ("username", "ubuntu"). The first occurrence of a label wins.
// from is the row to start at, so a later block (a VPN section) can be read on its own.
func (s Sheet) Pairs(from int) map[string]string {
	m := map[string]string{}
	for _, row := range s.Rows[min(from, len(s.Rows)):] {
		for i := 0; i+1 < len(row); i++ {
			k := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(row[i]), ":")))
			if _, seen := m[k]; k != "" && row[i+1] != "" && !seen {
				m[k] = row[i+1]
			}
		}
	}
	return m
}

// RowOf returns the first row containing a cell equal (ignoring case) to text, or -1.
func (s Sheet) RowOf(text string) int {
	for i, row := range s.Rows {
		for _, c := range row {
			if strings.EqualFold(strings.TrimSpace(c), text) {
				return i
			}
		}
	}
	return -1
}
