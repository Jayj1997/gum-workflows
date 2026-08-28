package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Jayj1997/gum-workflows/internal/execution"
)

type stdinHumanGateway struct {
	in  *bufio.Reader
	out io.Writer
}

func newStdinHumanGateway(in io.Reader, out io.Writer) *stdinHumanGateway {
	return &stdinHumanGateway{in: bufio.NewReader(in), out: out}
}

func (g *stdinHumanGateway) RequestRound(ctx context.Context, req execution.RoundRequest) (execution.RoundResponse, error) {
	if req.Kind != execution.RoundRequestInput {
		return execution.RoundResponse{}, fmt.Errorf("unsupported human request kind %q", req.Kind)
	}

	type result struct {
		response execution.RoundResponse
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		response, err := g.requestInput(req)
		resultCh <- result{response: response, err: err}
	}()

	select {
	case <-ctx.Done():
		return execution.RoundResponse{}, ctx.Err()
	case result := <-resultCh:
		return result.response, result.err
	}
}

func (g *stdinHumanGateway) requestInput(req execution.RoundRequest) (execution.RoundResponse, error) {
	if _, err := fmt.Fprintf(g.out, "\n[%s] Enter requirement; a blank line ends this round:\n", req.NodeID); err != nil {
		return execution.RoundResponse{}, fmt.Errorf("write human input prompt: %w", err)
	}

	var lines []string
	for {
		line, err := g.readLine()
		if err != nil {
			return execution.RoundResponse{}, fmt.Errorf("read human input: %w", err)
		}
		if line == "" {
			break
		}
		lines = append(lines, line)
	}

	for {
		if _, err := fmt.Fprint(g.out, "Continue or Finish? [c/F]: "); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write human decision prompt: %w", err)
		}
		decision, err := g.readLine()
		if err != nil {
			return execution.RoundResponse{}, fmt.Errorf("read human decision: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(decision)) {
		case "c", "continue":
			return execution.RoundResponse{Content: strings.Join(lines, "\n")}, nil
		case "", "f", "finish":
			return execution.RoundResponse{Content: strings.Join(lines, "\n"), Finished: true}, nil
		default:
			if _, err := fmt.Fprintln(g.out, "Please enter Continue or Finish."); err != nil {
				return execution.RoundResponse{}, fmt.Errorf("write human decision error: %w", err)
			}
		}
	}
}

func (g *stdinHumanGateway) readLine() (string, error) {
	line, err := g.in.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}
