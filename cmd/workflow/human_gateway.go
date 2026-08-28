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
	if req.Kind != execution.RoundRequestInput && req.Kind != execution.RoundRequestApproval {
		return execution.RoundResponse{}, fmt.Errorf("unsupported human request kind %q", req.Kind)
	}

	type result struct {
		response execution.RoundResponse
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		var response execution.RoundResponse
		var err error
		switch req.Kind {
		case execution.RoundRequestInput:
			response, err = g.requestInput(req)
		case execution.RoundRequestApproval:
			response, err = g.requestApproval(req)
		}
		resultCh <- result{response: response, err: err}
	}()

	select {
	case <-ctx.Done():
		return execution.RoundResponse{}, ctx.Err()
	case result := <-resultCh:
		return result.response, result.err
	}
}

func (g *stdinHumanGateway) requestApproval(req execution.RoundRequest) (execution.RoundResponse, error) {
	if _, err := fmt.Fprintf(g.out, "\n[%s] Artifacts produced in this run:\n", req.NodeID); err != nil {
		return execution.RoundResponse{}, fmt.Errorf("write approval context: %w", err)
	}
	if len(req.Artifacts) == 0 {
		if _, err := fmt.Fprintln(g.out, "  (none)"); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write approval artifacts: %w", err)
		}
	}
	for _, item := range req.Artifacts {
		if _, err := fmt.Fprintf(g.out, "  - %s | %s | v%s | %s\n", item.Name, item.Kind, item.Version, item.URI); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write approval artifact: %w", err)
		}
	}
	if _, err := fmt.Fprintln(g.out, "Advise history:"); err != nil {
		return execution.RoundResponse{}, fmt.Errorf("write advise history heading: %w", err)
	}
	if len(req.AdviseHistory) == 0 {
		if _, err := fmt.Fprintln(g.out, "  (none)"); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write advise history: %w", err)
		}
	}
	for _, advise := range req.AdviseHistory {
		if _, err := fmt.Fprintf(g.out, "  - %s\n", advise); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write advise history: %w", err)
		}
	}

	for {
		if _, err := fmt.Fprint(g.out, "Approve / Reject (with advise)? [A/r]: "); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write approval prompt: %w", err)
		}
		line, err := g.readLine()
		if err != nil {
			return execution.RoundResponse{}, fmt.Errorf("read approval decision: %w", err)
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.EqualFold(trimmed, "a") || strings.EqualFold(trimmed, "approve") {
			return execution.RoundResponse{Approved: true}, nil
		}
		choice, advise, _ := strings.Cut(trimmed, " ")
		if strings.EqualFold(choice, "r") || strings.EqualFold(choice, "reject") {
			advise = strings.TrimSpace(advise)
			if advise == "" {
				if _, err := fmt.Fprint(g.out, "Advise (optional): "); err != nil {
					return execution.RoundResponse{}, fmt.Errorf("write reject advise prompt: %w", err)
				}
				advise, err = g.readLine()
				if err != nil {
					return execution.RoundResponse{}, fmt.Errorf("read reject advise: %w", err)
				}
			}
			return execution.RoundResponse{Approved: false, Advise: strings.TrimSpace(advise)}, nil
		}
		if _, err := fmt.Fprintln(g.out, "Please enter Approve or Reject."); err != nil {
			return execution.RoundResponse{}, fmt.Errorf("write approval decision error: %w", err)
		}
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
