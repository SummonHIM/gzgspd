package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/summonhim/gzgspd/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf(
			"gzgspd %s %s %s with %s %s\n",
			version.Value,
			runtime.GOOS,
			runtime.GOARCH,
			runtime.Version(),
			version.BuildTime,
		)
		os.Exit(0)
	}
	fmt.Println("use internal/app")
	os.Exit(0)
}
