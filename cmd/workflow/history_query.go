package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/execution"
	"github.com/Jayj1997/gum-workflows/internal/history"
	"github.com/Jayj1997/gum-workflows/internal/runtimepath"
)

func historyCmd(ctx context.Context, args []string, paths runtimepath.Paths, out io.Writer) error {
	if len(args) > 2 {
		return fmt.Errorf("usage: workflow history [<run-id> [<node-id>]]")
	}
	if _, err := os.Stat(paths.Database()); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(out, "no runs recorded")
			return nil
		}
		return fmt.Errorf("stat history database: %w", err)
	}
	store, err := history.OpenReadOnly(ctx, paths.Database())
	if err != nil {
		return fmt.Errorf("open history database: %w", err)
	}
	defer store.Close()
	version, err := store.UserVersion(ctx)
	if err != nil {
		return fmt.Errorf("read history database schema version: %w", err)
	}
	if version < history.RunHistorySchemaVersion {
		fmt.Fprintln(out, "no runs recorded")
		return nil
	}

	if len(args) == 0 {
		runs, err := store.ListRuns(ctx)
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			fmt.Fprintln(out, "no runs recorded")
			return nil
		}
		printHistoryList(out, runs)
		return nil
	}
	if len(args) == 1 {
		run, err := store.GetRun(ctx, args[0])
		if err != nil {
			return err
		}
		if run == nil {
			fmt.Fprintf(out, "run %s not found\n", args[0])
			return nil
		}
		printRunDetail(out, paths, run)
		return nil
	}
	nodeRun, err := store.GetNodeRun(ctx, args[0], args[1])
	if err != nil {
		return err
	}
	if nodeRun == nil {
		fmt.Fprintf(out, "node %q not found in run %s\n", args[1], args[0])
		return nil
	}
	printNodeDetail(out, nodeRun)
	return nil
}

func printHistoryList(out io.Writer, runs []history.RunSummary) {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "RUN ID\tWORKFLOW\tSTATUS\tSTARTED\tDURATION\tNODES")
	for _, run := range runs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d/%d\n",
			shortRunID(run.ID), run.Workflow, run.Status, formatHistoryTime(run.StartedAt),
			formatHistoryDuration(run.StartedAt, run.FinishedAt), run.NodesCompleted, run.NodesTotal)
	}
	_ = w.Flush()
}

func printRunDetail(out io.Writer, paths runtimepath.Paths, run *history.RunDetail) {
	fmt.Fprintf(out, "Run %s\n", run.ID)
	workflowName := run.Workflow
	if run.WorkflowVersion != "" {
		workflowName += " " + run.WorkflowVersion
	}
	fmt.Fprintf(out, "  Workflow:       %s\n", workflowName)
	fmt.Fprintf(out, "  Status:         %s\n", run.Status)
	fmt.Fprintf(out, "  Started:        %s\n", formatHistoryTime(run.StartedAt))
	fmt.Fprintf(out, "  Finished:       %s\n", formatHistoryTime(run.FinishedAt))
	fmt.Fprintf(out, "  Duration:       %s\n", formatHistoryDuration(run.StartedAt, run.FinishedAt))
	fmt.Fprintf(out, "  File:           %s\n", emptyHistoryValue(run.WorkflowFile))
	fmt.Fprintf(out, "  State dir:      %s\n", runStateDir(paths, run.ID))
	if run.StoppedReason != "" {
		fmt.Fprintf(out, "  Stopped reason: %s\n", run.StoppedReason)
	}
	if run.Error != "" {
		fmt.Fprintf(out, "  Error:          %s\n", run.Error)
	}

	fmt.Fprintln(out, "\nNodes:")
	if len(run.Nodes) == 0 {
		fmt.Fprintln(out, "  no nodes recorded")
		return
	}
	for _, item := range run.Nodes {
		fmt.Fprintf(out, "  %s  %s %s  %s  rounds: %d  duration: %s  inputs: %d  outputs: %d",
			item.NodeID, item.NodeDefinition, item.NodeExecutor, item.Status, item.Rounds,
			formatHistoryDuration(item.StartedAt, item.FinishedAt), item.Inputs, item.Outputs)
		if item.ErrorKind != "" {
			fmt.Fprintf(out, "  error_kind: %s", item.ErrorKind)
		}
		if item.Error != "" {
			fmt.Fprintf(out, "  error: %s", item.Error)
		}
		fmt.Fprintln(out)
		for _, round := range item.RoundDetails {
			fmt.Fprintf(out, "    Round %d %s  duration: %s  inputs: %d  outputs: %d",
				round.Round, round.Status, formatHistoryDuration(round.StartedAt, round.FinishedAt),
				round.Inputs, round.Outputs)
			if round.ErrorKind != "" {
				fmt.Fprintf(out, "  error_kind: %s", round.ErrorKind)
			}
			if round.Error != "" {
				fmt.Fprintf(out, "  error: %s", round.Error)
			}
			fmt.Fprintln(out)
		}
	}
}

func printNodeDetail(out io.Writer, detail *history.NodeDetail) {
	fmt.Fprintf(out, "Run %s\n", detail.RunID)
	fmt.Fprintf(out, "Node %s (%s %s)\n", detail.NodeID, detail.NodeDefinition, detail.NodeExecutor)
	latest := detail.Rounds[len(detail.Rounds)-1]
	fmt.Fprintf(out, "Latest round: %d  %s  duration: %s", latest.Round, latest.Status,
		formatHistoryDuration(latest.StartedAt, latest.FinishedAt))
	if latest.ErrorKind != "" {
		fmt.Fprintf(out, "  error_kind: %s", latest.ErrorKind)
	}
	if latest.Error != "" {
		fmt.Fprintf(out, "  error: %s", latest.Error)
	}
	fmt.Fprintln(out, "\n\nRounds:")
	for _, round := range detail.Rounds {
		fmt.Fprintf(out, "  Round %d  %s  started: %s  duration: %s",
			round.Round, round.Status, formatHistoryTime(round.StartedAt),
			formatHistoryDuration(round.StartedAt, round.FinishedAt))
		if round.ErrorKind != "" {
			fmt.Fprintf(out, "  error_kind: %s", round.ErrorKind)
		}
		if round.Error != "" {
			fmt.Fprintf(out, "  error: %s", round.Error)
		}
		fmt.Fprintln(out)
		printRoundInputs(out, round.Inputs)
		printRoundOutputs(out, round.Outputs)
	}
}

func printRoundInputs(out io.Writer, inputs map[string]execution.InputSnapshot) {
	if len(inputs) == 0 {
		fmt.Fprintln(out, "    Inputs: -")
		return
	}
	fmt.Fprintln(out, "    Inputs:")
	for _, name := range sortedHistoryKeys(inputs) {
		input := inputs[name]
		fmt.Fprintf(out, "      %s  from: %s  kind: %s  uri: %s  version: %s\n",
			name, input.From, input.Ref.Kind, input.Ref.URI, input.Ref.Version)
	}
}

func printRoundOutputs(out io.Writer, outputs map[string]artifact.ArtifactRef) {
	if len(outputs) == 0 {
		fmt.Fprintln(out, "    Outputs: -")
		return
	}
	fmt.Fprintln(out, "    Outputs:")
	for _, name := range sortedHistoryKeys(outputs) {
		ref := outputs[name]
		fmt.Fprintf(out, "      %s  kind: %s  uri: %s  version: %s\n",
			name, ref.Kind, ref.URI, ref.Version)
	}
}

func sortedHistoryKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func runStateDir(paths runtimepath.Paths, runID string) string {
	if runID == "" {
		return "-"
	}
	return paths.RunDir(runID)
}

func emptyHistoryValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func shortRunID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func formatHistoryTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func formatHistoryDuration(started, finished time.Time) string {
	if started.IsZero() || finished.IsZero() {
		return "-"
	}
	return finished.Sub(started).Round(time.Millisecond).String()
}
