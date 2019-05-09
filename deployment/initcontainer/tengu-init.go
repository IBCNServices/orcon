package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	requiredVars := os.Getenv("TENGU_REQUIRED_VARS")
	for _, requiredVar := range strings.Split(requiredVars, ",") {
		if _, ok := os.LookupEnv(requiredVar); !ok {
			fmt.Printf("Var not found (%v) -> BLOCKING\n", requiredVar)

			// Sleeping forever
			// https://stackoverflow.com/a/36419288/1588555
			exitSignal := make(chan os.Signal)
			signal.Notify(exitSignal, syscall.SIGINT, syscall.SIGTERM)
			<-exitSignal
		}
	}
	fmt.Println("All variables found; shutting down..")
}
