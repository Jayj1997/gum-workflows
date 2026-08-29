package main

import "os"

func commandStdinIsInteractive() bool {
	return stdinIsTerminal(os.Stdin)
}
