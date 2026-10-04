// Command dumptree prints the named-node tree lgtm builds for each source
// file in a directory, with each node's field name and short text. It is a
// development aid for writing per-language tables.
//
// Usage:
//
//	go run ./cmd/dumptree <dir>
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/donaldwasserman/lgtm/internal/model"
	"github.com/donaldwasserman/lgtm/internal/parse"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dumptree <dir>")
		os.Exit(2)
	}
	files, err := parse.Scan(context.Background(), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := files[p]
		fmt.Printf("== %s (%s)\n", p, f.Lang)
		if f.Error != nil {
			fmt.Printf("   error: %s\n", f.Error.Msg)
		}
		dump(f.Root, 0)
	}
}

func dump(n *model.Node, indent int) {
	if n == nil {
		return
	}
	label := n.Type
	if n.Field != "" {
		label = n.Field + ": " + label
	}
	text := ""
	if len(n.Children) == 0 {
		text = " " + strings.ReplaceAll(n.Text, "\n", `\n`)
		if len(text) > 50 {
			text = text[:50] + "..."
		}
	}
	fmt.Printf("%s%s%s\n", strings.Repeat("  ", indent), label, text)
	for _, c := range n.Children {
		dump(c, indent+1)
	}
}
