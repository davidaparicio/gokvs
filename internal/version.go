/*
Copyright © 2023 David Aparicio david.aparicio@free.fr
*/
package internal

import "fmt"

// Version GitCommit BuiltDate are set at build-time
var Version = "v0.0.1-SNAPSHOT"
var GitCommit = "54a8d74ea3cf6fdcadfac10ee4a4f2553d4562f6q"
var BuildDate = "Thu Jan  1 01:00:00 CET 1970" // date -r 0 (Mac), date -d @0 (Linux)

func PrintVersion() {
	fmt.Printf("Server: \tGoKVs - Community\nVersion: \t%s\nGit commit: \t%s\nBuilt: \t\t%s\n", Version, GitCommit, BuildDate)
}
