package diagnostics

import "fmt"

type Stage string

const (
	StageLex       Stage = "lex"
	StageParse     Stage = "parse"
	StageLoad      Stage = "load"
	StageCheck     Stage = "check"
	StageInvariant Stage = "invariant"
	StageCodegen   Stage = "codegen"
	StageRuntime   Stage = "runtime"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Location struct {
	File      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
}

type Diagnostic struct {
	Code     string
	Stage    Stage
	Severity Severity
	Message  string
	Location *Location
	Notes    []string
}

func (d Diagnostic) String() string {
	severity := string(d.Severity)
	if severity == "" {
		severity = string(SeverityError)
	}
	codePart := ""
	if d.Code != "" {
		codePart = fmt.Sprintf("[%s]", d.Code)
	}
	stagePart := ""
	if d.Stage != "" {
		stagePart = fmt.Sprintf(" (%s)", d.Stage)
	}

	locPart := ""
	if d.Location != nil {
		line := d.Location.Line
		col := d.Location.Column
		if d.Location.File != "" && line > 0 && col > 0 {
			locPart = fmt.Sprintf(" at %s:%d:%d", d.Location.File, line, col)
		} else if d.Location.File != "" && line > 0 {
			locPart = fmt.Sprintf(" at %s:%d", d.Location.File, line)
		} else if line > 0 && col > 0 {
			locPart = fmt.Sprintf(" at line %d, col %d", line, col)
		} else if line > 0 {
			locPart = fmt.Sprintf(" at line %d", line)
		}
	}

	return fmt.Sprintf("%s%s%s: %s%s", severity, codePart, stagePart, d.Message, locPart)
}

func WithDefaultFile(diags []Diagnostic, file string) []Diagnostic {
	if file == "" || len(diags) == 0 {
		return diags
	}
	result := make([]Diagnostic, len(diags))
	for i, d := range diags {
		result[i] = d
		if result[i].Location != nil && result[i].Location.File == "" {
			loc := *result[i].Location
			loc.File = file
			result[i].Location = &loc
		}
	}
	return result
}
