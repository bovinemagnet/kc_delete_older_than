package main

import (
	"fmt"
	"log"
	"os"
)

type LogLevel int

const (
	INFO LogLevel = iota
	WARNING
	ERROR
)

func output(logLevel LogLevel, logFlag bool, printFlag bool, format string, a ...interface{}) {
	message := fmt.Sprintf(format, a...)
	switch logLevel {
	case INFO:
		if logFlag {
			log.Printf("[INFO] %s", message)
		}
		if printFlag {
			fmt.Printf("[INFO] %s\n", message)
		}
	case WARNING:
		if logFlag {
			log.Printf("[WARNING] %s", message)
		}
		if printFlag {
			fmt.Printf("[WARNING] %s\n", message)
		}
	case ERROR:
		if logFlag {
			log.Printf("[ERROR] %s", message)
		}
		if printFlag {
			fmt.Fprintf(os.Stderr, "[ERROR] %s\n", message)
		}
	}
}
