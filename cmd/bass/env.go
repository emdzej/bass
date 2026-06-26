package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		fmt.Fprintf(os.Stderr, "ignoring invalid duration in %s\n", k)
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		fmt.Fprintf(os.Stderr, "ignoring invalid integer in %s\n", k)
	}
	return def
}

func envBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		switch v {
		case "1", "true", "TRUE", "True", "yes":
			return true
		case "0", "false", "FALSE", "False", "no":
			return false
		}
	}
	return def
}
