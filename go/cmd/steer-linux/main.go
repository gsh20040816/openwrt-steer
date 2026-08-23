// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
)

var version = "development"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if len(os.Args) < 2 || os.Args[1] != "apply" {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
