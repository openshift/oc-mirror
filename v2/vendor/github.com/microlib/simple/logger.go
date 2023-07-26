package simple

import (
	"log"
)

const (
	INFO  = "info"
	DEBUG = "debug"
	WARN  = "warn"
	ERROR = "error"
	TRACE = "trace"
)

type Logger struct {
	Level string
}

func (logger Logger) Debug(message string) {
	if logger.Level == DEBUG || logger.Level == TRACE {
		log.Printf("\x1b[1;92m [%s] \x1b[0m : %s", "DEBUG", message)
	}
}

func (logger Logger) Error(message string) {
	log.Printf("\x1b[1;91m [%s] \x1b[0m : %s", "ERROR", message)
}

func (logger Logger) Warn(message string) {
	if logger.Level == WARN || logger.Level == DEBUG || logger.Level == INFO || logger.Level == TRACE {
		log.Printf("\x1b[1;93m [%s] \x1b[0m  : %s", "WARN", message)
	}
}

func (logger Logger) Trace(message string) {
	if logger.Level == TRACE {
		log.Printf("\x1b[1;96m [%s] \x1b[0m : %s", "TRACE", message)
	}
}

func (logger Logger) Info(message string) {
	if logger.Level == INFO || logger.Level == DEBUG || logger.Level == TRACE {
		log.Printf("\x1b[1;94m [%s] \x1b[0m  : %s", "INFO", message)
	}
}
