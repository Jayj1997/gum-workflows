package scriptnode

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/node"
)

func TestRaceAdapterBuildsHonestPassedRaceFailureAndNotApplicableResults(t *testing.T) {
	tests := []struct {
		name, packages, events, stderr string
		exitCode                       int
		verdict                        Verdict
		races                          *int
		findingKind                    RaceFindingKind
	}{
		{
			name: "no race observed", packages: "example.com/app\n",
			events: `{"Action":"run","Package":"example.com/app","Test":"TestSafe"}` + "\n" +
				`{"Action":"pass","Package":"example.com/app","Test":"TestSafe"}` + "\n" +
				`{"Action":"pass","Package":"example.com/app"}` + "\n",
			verdict: VerdictPassed, races: intPointer(0),
		},
		{
			name: "race observed", packages: "example.com/app\n", exitCode: 1,
			events: `{"Action":"run","Package":"example.com/app","Test":"TestRace"}` + "\n" +
				`{"Action":"output","Package":"example.com/app","Test":"TestRace","Output":"WARNING: DATA RACE\\n"}` + "\n" +
				`{"Action":"output","Package":"example.com/app","Test":"TestRace","Output":"race detected during execution of test\\n"}` + "\n" +
				`{"Action":"fail","Package":"example.com/app","Test":"TestRace"}` + "\n" +
				`{"Action":"fail","Package":"example.com/app"}` + "\n",
			verdict: VerdictFailed, races: intPointer(1), findingKind: RaceFindingObserved,
		},
		{
			name: "ordinary test failure", packages: "example.com/app\n", exitCode: 1,
			events: `{"Action":"run","Package":"example.com/app","Test":"TestBroken"}` + "\n" +
				`{"Action":"output","Package":"example.com/app","Test":"TestBroken","Output":"app_test.go:9: wrong value\\n"}` + "\n" +
				`{"Action":"fail","Package":"example.com/app","Test":"TestBroken"}` + "\n" +
				`{"Action":"fail","Package":"example.com/app"}` + "\n",
			verdict: VerdictFailed, races: intPointer(0), findingKind: RaceFindingTestFailure,
		},
		{
			name: "compile failure", packages: "example.com/app\n", exitCode: 1,
			events: `{"Action":"output","Package":"example.com/app","Output":"./app.go:4: undefined: missing\\n"}` + "\n" +
				`{"Action":"fail","Package":"example.com/app"}` + "\n",
			verdict: VerdictFailed, races: intPointer(0), findingKind: RaceFindingCompileFailure,
		},
		{
			name: "package loading failure", exitCode: 1, stderr: "pattern ./...: directory prefix . does not contain main module\n",
			verdict: VerdictFailed, races: intPointer(0), findingKind: RaceFindingPackageFailure,
		},
		{name: "no Go package", verdict: VerdictNotApplicable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := AdaptRaceResult(raceExecutionFixture(t, tt.packages, tt.events, tt.stderr, tt.exitCode))
			if err != nil {
				t.Fatalf("AdaptRaceResult() unexpected error: %v", err)
			}
			metric := result.Metrics.RacesDetected
			if result.Verdict != tt.verdict {
				t.Fatalf("verdict = %s, want %s", result.Verdict, tt.verdict)
			}
			if tt.races == nil {
				if metric.Available || !strings.Contains(metric.Reason, "package") {
					t.Fatalf("metric = %+v, want unavailable no-package reason", metric)
				}
			} else if !metric.Available || metric.Value == nil || *metric.Value != *tt.races {
				t.Fatalf("metric = %+v, want %d observed races", metric, *tt.races)
			}
			if tt.findingKind != "" && (len(result.Findings) == 0 || result.Findings[0].Kind != tt.findingKind) {
				t.Fatalf("findings = %+v, want kind %q", result.Findings, tt.findingKind)
			}
			if err := result.Validate(); err != nil {
				t.Errorf("result contract: %v", err)
			}
		})
	}
}

func TestRaceAdapterRejectsDamagedOrInfrastructureEvidence(t *testing.T) {
	tests := []struct {
		name, events string
		exitCode     int
		reportedExit *int
	}{
		{name: "invalid Go JSON", events: "{not-json}\n", exitCode: 1},
		{name: "empty successful Go JSON", exitCode: 0},
		{name: "meaningless successful Go JSON", events: "{}\n", exitCode: 0},
		{name: "infrastructure exit", exitCode: 125},
		{name: "mismatched exit", exitCode: 1, reportedExit: intPointer(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := raceExecutionFixture(t, "example.com/app\n", tt.events, "", tt.exitCode)
			if tt.reportedExit != nil {
				writeFixture(t, filepath.Join(record.ToolOutputDir, "test-exit.txt"), fmt.Sprintf("%d\n", *tt.reportedExit))
			}
			_, err := AdaptRaceResult(record)
			if err == nil || node.ErrorKindOf(err) != node.ErrorKindStructural {
				t.Fatalf("AdaptRaceResult() error = %v, want Structural Error", err)
			}
		})
	}
}

func TestRaceAdapterClassifiesFailurePerPackage(t *testing.T) {
	events := `{"Action":"run","Package":"example.com/safe","Test":"TestSafe"}` + "\n" +
		`{"Action":"pass","Package":"example.com/safe","Test":"TestSafe"}` + "\n" +
		`{"Action":"pass","Package":"example.com/safe"}` + "\n" +
		`{"Action":"output","Package":"example.com/broken","Output":"./broken.go:4: undefined: missing\\n"}` + "\n" +
		`{"Action":"fail","Package":"example.com/broken"}` + "\n"
	result, err := AdaptRaceResult(raceExecutionFixture(t, "example.com/safe\nexample.com/broken\n", events, "", 1))
	if err != nil {
		t.Fatalf("AdaptRaceResult() unexpected error: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Package != "example.com/broken" || result.Findings[0].Kind != RaceFindingCompileFailure {
		t.Fatalf("findings = %+v, want compile failure for broken package", result.Findings)
	}
}

func raceExecutionFixture(t *testing.T, packages, events, stderr string, exitCode int) ExecutionRecord {
	t.Helper()
	record := staticExecutionFixture(t, packages, "", stderr, exitCode)
	writeFixture(t, filepath.Join(record.ToolOutputDir, "cc.txt"), "clang\n")
	writeFixture(t, filepath.Join(record.ToolOutputDir, "test.json"), events)
	writeFixture(t, filepath.Join(record.ToolOutputDir, "test-exit.txt"), fmt.Sprintf("%d\n", exitCode))
	return record
}

func intPointer(value int) *int { return &value }
