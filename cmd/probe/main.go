/*
SPDX-FileCopyrightText: 2026 Rillan AI LLC
SPDX-License-Identifier: MIT
*/

// probe is the tiny in-cluster network probe binary used by Fathom probe pods.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

const terminationLog = "/dev/termination-log"

type result struct {
	Outcome string            `json:"outcome"`
	Summary string            `json:"summary"`
	Details map[string]string `json:"details,omitempty"`
}

func main() {
	if err := run(); err != nil {
		writeResult(result{Outcome: "Error", Summary: err.Error()})
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "probe mode: dns, tcp-connect, tcp-listen")
	target := flag.String("target", "", "DNS name or TCP host")
	port := flag.Int("port", 0, "TCP port")
	timeout := flag.Duration("timeout", 10*time.Second, "probe timeout")
	listenAddress := flag.String("listen-address", "0.0.0.0", "address for tcp-listen")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch *mode {
	case "dns":
		return runDNS(ctx, *target)
	case "tcp-connect":
		return runTCPConnect(ctx, *target, *port)
	case "tcp-listen":
		return runTCPListen(ctx, *listenAddress, *port)
	default:
		return fmt.Errorf("unsupported probe mode %q", *mode)
	}
}

func runDNS(ctx context.Context, target string) error {
	if target == "" {
		return errors.New("dns probe target is required")
	}
	started := time.Now()
	addresses, err := net.DefaultResolver.LookupHost(ctx, target)
	latency := time.Since(started)
	details := map[string]string{"target": target, "latencyMillis": strconv.FormatInt(latency.Milliseconds(), 10)}
	if err != nil {
		details["error"] = err.Error()
		writeResult(result{Outcome: "Error", Summary: "DNS resolution failed", Details: details})
		return err
	}
	if len(addresses) == 0 {
		writeResult(result{Outcome: "Fail", Summary: "DNS resolution returned no addresses", Details: details})
		return nil
	}
	details["addresses"] = join(addresses)
	writeResult(result{Outcome: "Pass", Summary: "DNS resolution succeeded", Details: details})
	return nil
}

func runTCPConnect(ctx context.Context, target string, port int) error {
	if target == "" {
		return errors.New("tcp-connect probe target is required")
	}
	if port <= 0 {
		return errors.New("tcp-connect probe port is required")
	}
	started := time.Now()
	dialer := net.Dialer{}
	address := net.JoinHostPort(target, strconv.Itoa(port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	latency := time.Since(started)
	details := map[string]string{"target": target, "port": strconv.Itoa(port), "latencyMillis": strconv.FormatInt(latency.Milliseconds(), 10)}
	if err != nil {
		details["error"] = err.Error()
		writeResult(result{Outcome: "Fail", Summary: "TCP connection failed", Details: details})
		return nil
	}
	_ = conn.Close()
	writeResult(result{Outcome: "Pass", Summary: "TCP connection succeeded", Details: details})
	return nil
}

func runTCPListen(ctx context.Context, listenAddress string, port int) error {
	if port <= 0 {
		return errors.New("tcp-listen probe port is required")
	}
	address := net.JoinHostPort(listenAddress, strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		writeResult(result{Outcome: "Error", Summary: "TCP listener failed", Details: map[string]string{"address": address, "error": err.Error()}})
		return err
	}
	defer func() { _ = listener.Close() }()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				writeResult(result{Outcome: "Pass", Summary: "TCP listener completed", Details: map[string]string{"address": address}})
				return nil
			}
			writeResult(result{Outcome: "Error", Summary: "TCP listener failed", Details: map[string]string{"address": address, "error": err.Error()}})
			return err
		}
		_ = conn.Close()
	}
}

func writeResult(r result) {
	encoded, err := json.Marshal(r)
	if err != nil {
		encoded = []byte(`{"outcome":"Error","summary":"failed to encode probe result"}`)
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
	_ = os.WriteFile(terminationLog, encoded, 0o644)
}

func join(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
