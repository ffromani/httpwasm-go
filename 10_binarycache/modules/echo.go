package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	_, err := io.Copy(os.Stdout, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}
