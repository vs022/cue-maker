package main

import (
	"fmt"
	"os"
)

func checkPanic() {
	if r := recover(); r != nil {
		switch r.(type) {
		case string:
			logErrorMessage(r.(string))
			os.Exit(1)
		default:
			panic(r)
		}
	}
}

func panicIfError(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func logIfError(err error) {
	if err != nil {
		logErrorMessage(err.Error())
	}
}

func logErrorMessage(msg string) {
	if msg != "" {
		logMessage("Error: " + msg)
	}
}

func logMessage(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}
