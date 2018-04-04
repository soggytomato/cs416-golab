package main

import "os/exec"

func main() {
    commands := []string{
        "rm -rf log*",
        "rm -rf session*",
        "rm FSNode_*-Log.txt",
        "rm FSServer-Log.txt",
        "rm nodeID"}

    for _, cmd := range commands {
        exec.Command("/bin/sh", "-c", cmd).Run()
    }
}