package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"sophia/internal/model"
	"strconv"
	"strings"
)

const (
	evidenceTypeCommandRun       = "command_run"
	evidenceTypeManualNote       = "manual_note"
	evidenceTypeEnvironment      = "environment"
	evidenceTypeBenchmark        = "benchmark"
	evidenceTypeReproductionStep = "reproduction_steps"
	evidenceTypeReviewSample     = "review_sample"
)

type AddEvidenceOptions struct {
	Type        string
	Scope       string
	Summary     string
	Command     string
	Capture     bool
	ExitCode    *int
	Attachments []string
}

func normalizeEvidenceType(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case evidenceTypeCommandRun, evidenceTypeManualNote, evidenceTypeEnvironment, evidenceTypeBenchmark, evidenceTypeReproductionStep, evidenceTypeReviewSample:
		return value, nil
	default:
		return "", fmt.Errorf("%w: %q (allowed: %s, %s, %s, %s, %s, %s)", ErrInvalidEvidenceType, raw, evidenceTypeCommandRun, evidenceTypeManualNote, evidenceTypeEnvironment, evidenceTypeBenchmark, evidenceTypeReproductionStep, evidenceTypeReviewSample)
	}
}

func (s *Service) AddEvidence(id int, opts AddEvidenceOptions) (*model.EvidenceEntry, error) {
	evidenceType, err := normalizeEvidenceType(opts.Type)
	if err != nil {
		return nil, err
	}
	loadTargetCR := func() (*model.CR, error) {
		cr, loadErr := s.store.LoadCR(id)
		if loadErr != nil {
			return nil, loadErr
		}
		if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
			return nil, guardErr
		}
		return cr, nil
	}
	if _, err := loadTargetCR(); err != nil {
		return nil, err
	}

	command := strings.TrimSpace(opts.Command)
	summary := strings.TrimSpace(opts.Summary)
	attachments := []string{}
	if len(opts.Attachments) > 0 {
		normalized, normalizeErr := s.normalizeTaskScopePaths(opts.Attachments)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		attachments = normalized
	}

	var (
		exitCode   *int
		outputHash string
	)
	if opts.ExitCode != nil {
		exit := *opts.ExitCode
		exitCode = &exit
	}

	if opts.Capture && evidenceType != evidenceTypeCommandRun {
		return nil, fmt.Errorf("%w: --capture is only supported for %q evidence type", ErrInvalidEvidenceType, evidenceTypeCommandRun)
	}
	if opts.Capture && command == "" {
		return nil, fmt.Errorf("command is required when --capture is set")
	}
	if opts.Capture {
		capturedExitCode, capturedOutputHash, capturedSummary, captureErr := s.captureEvidenceCommand(command)
		if captureErr != nil {
			return nil, captureErr
		}
		exitCode = &capturedExitCode
		outputHash = capturedOutputHash
		if summary == "" {
			summary = capturedSummary
		}
	}
	if summary == "" {
		return nil, errors.New("summary cannot be empty")
	}

	var out model.EvidenceEntry
	if err := s.withMutationLock(func() error {
		cr, err := loadTargetCR()
		if err != nil {
			return err
		}

		now := s.timestamp()
		actor := s.git.Actor()
		entry := model.EvidenceEntry{
			TS:          now,
			Actor:       actor,
			Type:        evidenceType,
			Scope:       strings.TrimSpace(opts.Scope),
			Command:     command,
			ExitCode:    exitCode,
			OutputHash:  outputHash,
			Summary:     summary,
			Attachments: append([]string(nil), attachments...),
		}

		cr.Evidence = append(cr.Evidence, entry)
		meta := map[string]string{
			"type": evidenceType,
		}
		if entry.Scope != "" {
			meta["scope"] = entry.Scope
		}
		if entry.Command != "" {
			meta["command"] = entry.Command
		}
		if entry.OutputHash != "" {
			meta["output_hash"] = entry.OutputHash
		}
		if entry.ExitCode != nil {
			meta["exit_code"] = strconv.Itoa(*entry.ExitCode)
		}
		if len(entry.Attachments) > 0 {
			meta["attachments"] = strings.Join(entry.Attachments, ",")
			meta["attachments_count"] = strconv.Itoa(len(entry.Attachments))
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeEvidenceAdded,
			Summary: fmt.Sprintf("Added %s evidence: %s", evidenceType, truncateSummary(summary, 90)),
			Ref:     fmt.Sprintf("cr:%d", id),
			Meta:    meta,
		})
		cr.UpdatedAt = now

		if err := s.store.SaveCR(cr); err != nil {
			return err
		}
		out = entry
		out.Attachments = append([]string(nil), entry.Attachments...)
		return nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ListEvidence(id int) ([]model.EvidenceEntry, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	out := make([]model.EvidenceEntry, 0, len(cr.Evidence))
	for _, entry := range cr.Evidence {
		copied := entry
		copied.Attachments = append([]string(nil), entry.Attachments...)
		if entry.ExitCode != nil {
			exit := *entry.ExitCode
			copied.ExitCode = &exit
		}
		out = append(out, copied)
	}
	return out, nil
}

func (s *Service) captureEvidenceCommand(command string) (int, string, string, error) {
	cmd := exec.Command("sh", "-lc", command)
	cmd.Dir = s.git.WorkDir
	raw, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return 0, "", "", fmt.Errorf("capture command output: %w", err)
		}
	}

	sum := sha256.Sum256(raw)
	outputHash := hex.EncodeToString(sum[:])
	summary := buildCapturedCommandSummary(command, exitCode, string(raw))
	return exitCode, outputHash, summary, nil
}

func buildCapturedCommandSummary(command string, exitCode int, rawOutput string) string {
	command = strings.TrimSpace(command)
	commandLabel := truncateSummary(command, 80)
	base := fmt.Sprintf("command %q exited %d", commandLabel, exitCode)
	firstLine := firstNonEmptyLine(rawOutput)
	if firstLine == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", base, truncateSummary(firstLine, 120))
}

func firstNonEmptyLine(raw string) string {
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateSummary(input string, max int) string {
	input = strings.TrimSpace(input)
	if max <= 0 || len(input) <= max {
		return input
	}
	if max <= 3 {
		return input[:max]
	}
	return input[:max-3] + "..."
}
