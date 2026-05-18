package engine

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func Extract(sel *goquery.Selection, rule string) []string {
	parts := strings.Split(rule, "|")
	selector := strings.TrimSpace(parts[0])
	operation := "text"
	if len(parts) > 1 {
		operation = strings.TrimSpace(parts[1])
	}

	values := make([]string, 0)
	sel.Find(selector).Each(func(_ int, selection *goquery.Selection) {
		switch {
		case operation == "text":
			values = append(values, strings.TrimSpace(selection.Text()))
		case operation == "html":
			html, _ := selection.Html()
			values = append(values, strings.TrimSpace(html))
		case strings.HasPrefix(operation, "attr:"):
			attr := strings.TrimPrefix(operation, "attr:")
			if value, exists := selection.Attr(attr); exists {
				values = append(values, strings.TrimSpace(value))
			}
		}
	})
	return values
}
