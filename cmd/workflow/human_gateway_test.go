package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/execution"
)

func TestStdinHumanGatewayParsesInputRounds(t *testing.T) {
	tests := []struct {
		name         string
		stdin        string
		wantContent  string
		wantFinished bool
	}{
		{
			name:         "continue after multiline requirement",
			stdin:        "build an API\nwith tests\n\ncontinue\n",
			wantContent:  "build an API\nwith tests",
			wantFinished: false,
		},
		{
			name:         "empty decision defaults to finish",
			stdin:        "ship it\n\n\n",
			wantContent:  "ship it",
			wantFinished: true,
		},
		{
			name:         "invalid decision is prompted again",
			stdin:        "ship it\n\nmaybe\nfinish\n",
			wantContent:  "ship it",
			wantFinished: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			gateway := newStdinHumanGateway(strings.NewReader(tt.stdin), &stdout)
			response, err := gateway.RequestRound(context.Background(), execution.RoundRequest{
				NodeID: "input", Definition: "human-input", Kind: execution.RoundRequestInput,
			})
			if err != nil {
				t.Fatalf("RequestRound() unexpected error: %v", err)
			}
			if response.Content != tt.wantContent || response.Finished != tt.wantFinished {
				t.Errorf("response = %+v, want content %q/finished %v", response, tt.wantContent, tt.wantFinished)
			}
			output := stdout.String()
			for _, want := range []string{"input", "blank line", "Continue", "Finish"} {
				if !strings.Contains(output, want) {
					t.Errorf("prompt %q missing %q", output, want)
				}
			}
			if strings.Contains(tt.stdin, "maybe") && !strings.Contains(output, "enter Continue or Finish") {
				t.Errorf("invalid-choice prompt = %q", output)
			}
		})
	}
}

func TestStdinHumanGatewayParsesAdviseRetry(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		wantAdvise string
		wantSkip   bool
	}{
		{name: "non-empty advise retries", stdin: " return valid JSON \n", wantAdvise: "return valid JSON"},
		{name: "empty line skips", stdin: "\n", wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			gateway := newStdinHumanGateway(strings.NewReader(tt.stdin), &stdout)
			response, err := gateway.RequestRound(context.Background(), execution.RoundRequest{
				NodeID: "backend", Definition: "coding-agent", Kind: execution.RoundRequestAdviseRetry,
				Error: "expected JSON but received prose",
			})
			if err != nil {
				t.Fatalf("RequestRound() unexpected error: %v", err)
			}
			if response.Advise != tt.wantAdvise || response.Skip != tt.wantSkip {
				t.Errorf("response = %+v, want advise %q/skip %v", response, tt.wantAdvise, tt.wantSkip)
			}
			output := stdout.String()
			for _, want := range []string{"backend", "expected JSON but received prose", "advise", "empty line"} {
				if !strings.Contains(output, want) {
					t.Errorf("prompt %q missing %q", output, want)
				}
			}
		})
	}
}

func TestStdinHumanGatewayParsesApprovalAndShowsReviewContext(t *testing.T) {
	tests := []struct {
		name         string
		stdin        string
		wantApproved bool
		wantAdvise   string
	}{
		{name: "enter defaults to approve", stdin: "\n", wantApproved: true},
		{name: "reject with same-line advise", stdin: "r add tests\n", wantAdvise: "add tests"},
		{name: "reject with next-line advise", stdin: "r\nfix layout\n", wantAdvise: "fix layout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			gateway := newStdinHumanGateway(strings.NewReader(tt.stdin), &stdout)
			response, err := gateway.RequestRound(context.Background(), execution.RoundRequest{
				NodeID: "review", Definition: "human-approval", Kind: execution.RoundRequestApproval,
				Artifacts:     []execution.ArtifactSummary{{Name: "source-code", Kind: "SourceCode", Version: "2", URI: "mem://2"}},
				AdviseHistory: []string{"add error handling"},
			})
			if err != nil {
				t.Fatalf("RequestRound() unexpected error: %v", err)
			}
			if response.Approved != tt.wantApproved || response.Advise != tt.wantAdvise {
				t.Errorf("response = %+v, want approved %v/advise %q", response, tt.wantApproved, tt.wantAdvise)
			}
			output := stdout.String()
			for _, want := range []string{"review", "source-code", "SourceCode", "2", "mem://2", "add error handling", "Approve", "Reject", "[A/r]"} {
				if !strings.Contains(output, want) {
					t.Errorf("prompt %q missing %q", output, want)
				}
			}
		})
	}
}
