// Storecheck is for the store repository's CI and for plugin authors.
//
//	storecheck checksum DIR     prints the checksum of a plugin folder, for its index entry
//	storecheck index FILE       checks every entry: it downloads the pinned commit and verifies it
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Juan17la/lazycode/internal/store"
)

func main() {
	if len(os.Args) != 3 {
		fail("usage: storecheck checksum DIR | storecheck index FILE")
	}
	switch os.Args[1] {
	case "checksum":
		sum, err := store.Checksum(os.Args[2])
		if err != nil {
			fail(err)
		}
		fmt.Println(sum)
	case "index":
		if !checkIndex(os.Args[2]) {
			os.Exit(1)
		}
	default:
		fail("usage: storecheck checksum DIR | storecheck index FILE")
	}
}

func fail(v any) {
	fmt.Fprintln(os.Stderr, v)
	os.Exit(1)
}

func checkIndex(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	entries, problems, err := store.Parse(data)
	if err != nil {
		fail(err)
	}
	ok := len(problems) == 0
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "bad entry:", p)
	}
	seen := map[string]bool{}
	temp, err := os.MkdirTemp("", "storecheck-")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(temp) //nolint:errcheck // Temporary.
	client := store.New()
	for _, e := range entries {
		if seen[e.Name] {
			fmt.Fprintln(os.Stderr, "listed twice:", e.Name)
			ok = false
		}
		seen[e.Name] = true
		if _, _, err := client.Download(context.Background(), e, temp); err != nil {
			fmt.Fprintln(os.Stderr, "bad plugin:", err)
			ok = false
			continue
		}
		fmt.Println("ok:", e.Name, e.Version)
	}
	return ok
}
