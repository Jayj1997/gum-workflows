package execution

import (
	"context"
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// RoundRequestKind identifies the human interaction requested by a node round.
type RoundRequestKind string

const (
	// RoundRequestInput requests a new requirement from the workflow entry.
	RoundRequestInput RoundRequestKind = "input"
	// RoundRequestApproval requests an approval decision for completed work.
	RoundRequestApproval RoundRequestKind = "approval"
	// RoundRequestAdviseRetry requests advice for retrying an interaction error.
	RoundRequestAdviseRetry RoundRequestKind = "advise-retry"
)

// ArtifactSummary is the display-safe context shown before a human decision.
type ArtifactSummary struct {
	Name    string
	Kind    string
	Version string
	URI     string
}

// RoundRequest describes one blocking human interaction.
type RoundRequest struct {
	NodeID        string
	Definition    string
	Kind          RoundRequestKind
	Artifacts     []ArtifactSummary
	AdviseHistory []string
}

// RoundResponse carries the response fields for all supported request kinds.
type RoundResponse struct {
	Content  string
	Finished bool
	Approved bool
	Advise   string
	Skip     bool
}

// HumanGateway isolates the engine from terminal or UI interaction.
type HumanGateway interface {
	RequestRound(ctx context.Context, req RoundRequest) (RoundResponse, error)
}

// humanInputNode is implemented by the selected human-input executor. Keeping
// this interface in the consumer lets executor versions own output behavior
// without making builtins depend on execution.
type humanInputNode interface {
	node.Node
	ExecuteHumanInput(ctx node.ExecutionContext, content string) (map[string]artifact.ArtifactRef, error)
}

type noHumanGateway struct{}

func (noHumanGateway) RequestRound(context.Context, RoundRequest) (RoundResponse, error) {
	return RoundResponse{}, fmt.Errorf("human gateway is required for human node execution")
}
