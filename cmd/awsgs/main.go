// Package main provides awsgs, a thin wrapper around the aws CLI that automatically
// injects --endpoint-url so every command targets a running Gopherstack instance.
//
// Usage:
//
//	awsgs [--awsgs-port PORT] [--awsgs-host HOST] <aws-args...>
//
// Examples:
//
//	awsgs s3 ls
//	awsgs --awsgs-port 9000 sqs create-queue --queue-name my-queue
//	AWSGS_PORT=9000 awsgs dynamodb list-tables
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

const (
	version     = "1.0.0"
	defaultPort = "8000"
	defaultHost = "localhost"

	flagPort = "--awsgs-port"
	flagHost = "--awsgs-host"

	// endpointURLArgCount is the number of extra arguments added by buildArgs
	// when injecting --endpoint-url.
	endpointURLArgCount = 2

	envGopherstackPort = "GOPHERSTACK_PORT"
	flagEndpointURL    = "--endpoint-url"
)

func main() {
	os.Exit(run())
}

//nolint:gosec // awsgs intentionally shells out to aws CLI with caller-provided args.
func run() int {
	args := os.Args[1:]

	// Handle top-level --help / --version before anything else.
	for _, a := range args {
		switch a {
		case "--help", "-h":
			printHelp()

			return 0
		case "--version":
			fmt.Fprintln(os.Stdout, "awsgs", version)

			return 0
		}
	}

	host, port, rest := parseAwsgsFlags(args)

	endpoint := "http://" + net.JoinHostPort(host, port)
	awsArgs := buildArgs(rest, endpoint)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := exec.CommandContext(
		ctx,
		"aws",
		awsArgs...,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode()
		}

		fmt.Fprintf(os.Stderr, "awsgs: failed to run aws: %v\n", err)

		return 1
	}

	return 0
}

// parseAwsgsFlags strips --awsgs-port and --awsgs-host flags from args,
// returning the resolved host, port, and remaining args to forward to aws.
func parseAwsgsFlags(args []string) (string, string, []string) {
	host := defaultHost
	port := resolvePort()
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == flagPort:
			if i+1 < len(args) {
				port = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, flagPort+"="):
			port = strings.TrimPrefix(arg, flagPort+"=")
		case arg == flagHost:
			if i+1 < len(args) {
				host = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, flagHost+"="):
			host = strings.TrimPrefix(arg, flagHost+"=")
		default:
			rest = append(rest, arg)
		}
	}

	return host, port, rest
}

// resolvePort returns the port from environment variables or the default.
// Priority: AWSGS_PORT → GOPHERSTACK_PORT → defaultPort.
func resolvePort() string {
	for _, env := range []string{"AWSGS_PORT", envGopherstackPort} {
		if v := os.Getenv(env); v != "" {
			if _, err := strconv.Atoi(v); err != nil {
				fmt.Fprintf(os.Stderr, "awsgs: invalid port in %s, using default %s\n", env, defaultPort)

				return defaultPort
			}

			return v
		}
	}

	return defaultPort
}

// buildArgs inserts --endpoint-url <endpoint> at the front of aws CLI args.
// If --endpoint-url is already present it is left unchanged.
func buildArgs(args []string, endpoint string) []string {
	for _, a := range args {
		if a == flagEndpointURL || strings.HasPrefix(a, flagEndpointURL+"=") {
			// Already set — pass through as-is.
			return args
		}
	}

	result := make([]string, 0, len(args)+endpointURLArgCount)
	result = append(result, flagEndpointURL, endpoint)
	result = append(result, args...)

	return result
}

func printHelp() {
	fmt.Fprint(os.Stdout, `awsgs — AWS CLI wrapper for Gopherstack

USAGE
  awsgs [AWSGS-FLAGS] <aws service> <aws operation> [aws flags...]

AWSGS FLAGS
  --awsgs-port PORT   Port of the Gopherstack server (default: 8000)
  --awsgs-host HOST   Host of the Gopherstack server (default: localhost)
  --help              Show this help
  --version           Show version

ENVIRONMENT VARIABLES
  AWSGS_PORT          Port override (same as --awsgs-port)
  GOPHERSTACK_PORT    Port override (fallback, same precedence as AWSGS_PORT)

EXAMPLES
  awsgs s3 ls
  awsgs s3 mb s3://my-bucket
  awsgs sqs create-queue --queue-name my-queue
  awsgs --awsgs-port 9000 dynamodb list-tables
  awsgs s3 ls --endpoint-url http://custom:8080   # kept as-is

All remaining flags and arguments are forwarded verbatim to the aws CLI.
`)
}
