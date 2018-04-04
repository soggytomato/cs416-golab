package main

import "os/exec"

func main() {
    commands := []string{
        "rm *-Log.txt",
        "rm execute/*"}

    for _, cmd := range commands {
        exec.Command("/bin/sh", "-c", cmd).Run()
    }
}