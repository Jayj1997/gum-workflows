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

func TestStdinHumanGatewayRejectsUnsupportedRoundKind(t *testing.T) {
	gateway := newStdinHumanGateway(strings.NewReader(""), &bytes.Buffer{})
	_, err := gateway.RequestRound(context.Background(), execution.RoundRequest{Kind: execution.RoundRequestApproval})
	if err == nil || !strings.Contains(err.Error(), "unsupported human request kind") {
		t.Fatalf("RequestRound() error = %v, want unsupported kind", err)
	}
}
