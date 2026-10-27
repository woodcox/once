package command

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

// doctorExitError reports that at least one application failed a check. The
// doctor prints its own report, so the error carries no message.
type doctorExitError struct{}

func (e doctorExitError) Error() string {
	return "one or more checks failed"
}

func (e doctorExitError) ExitCode() int {
	return 1
}

type doctorCommand struct {
	cmd *cobra.Command
}

func newDoctorCommand() *doctorCommand {
	d := &doctorCommand{}
	d.cmd = &cobra.Command{
		Use:   "doctor [host]",
		Short: "Check an application's health",
		Args:  cobra.MaximumNArgs(1),
		RunE:  WithNamespace(d.run),
	}
	return d
}

// Private

func (d *doctorCommand) run(ctx context.Context, ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	result, err := docker.Doctor(ctx, ns, name, docker.DoctorProbesFor(ns))
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	renderDoctorReport(out, result)

	if result.Failed() {
		return doctorExitError{}
	}

	return nil
}

// renderDoctorReport writes the doctor's checklist.
func renderDoctorReport(out io.Writer, result docker.DoctorResult) {
	for _, report := range result.Reports {
		renderDoctorApp(out, report)
	}
}

// renderDoctorApp writes the checklist for a single application.
func renderDoctorApp(out io.Writer, report docker.DoctorReport) {
	heading := report.AppName
	if report.Host != "" {
		heading += " (" + report.Host + ")"
	}
	fmt.Fprintln(out, heading)

	for _, check := range report.Checks {
		fmt.Fprintf(out, "  %-5s %s\n", check.Status, doctorCheckLine(check))
		if check.Status == docker.DoctorFail && check.Remedy != "" {
			fmt.Fprintf(out, "        remedy: %s\n", check.Remedy)
		}
	}

	if report.URL != "" && !doctorHasURLCheck(report) {
		fmt.Fprintf(out, "  %-5s %s\n", "url", report.URL)
	}

	if report.Note != "" {
		fmt.Fprintf(out, "  note: %s\n", strings.TrimSpace(report.Note))
	}

	fmt.Fprintln(out)
}

// doctorCheckLine renders a check as its label followed by its message.
func doctorCheckLine(check docker.DoctorCheck) string {
	if check.Message == "" {
		return check.Label
	}
	if strings.HasPrefix(check.Message, ":") {
		return check.Label + check.Message
	}
	return check.Label + " " + check.Message
}

// doctorHasURLCheck reports whether the checklist already prints the app's
// URL, so it isn't printed twice.
func doctorHasURLCheck(report docker.DoctorReport) bool {
	for _, check := range report.Checks {
		if check.Label == "url" {
			return true
		}
	}
	return false
}
