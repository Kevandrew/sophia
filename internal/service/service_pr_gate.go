package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sophia/internal/model"
	"sort"
	"strconv"
	"strings"
)

const (
	prProviderGitHub = "github"
)

type PRContextView struct {
	CRID     int
	CRUID    string
	Title    string
	PRTitle  string
	Branch   string
	BaseRef  string
	Markdown string
	BodyHash string
	Warnings []string
}

type PRStatusView struct {
	CRID               int
	CRUID              string
	Provider           string
	Repo               string
	Number             int
	URL                string
	State              string
	Draft              bool
	ReviewDecision     string
	Merged             bool
	MergedAt           string
	MergedCommit       string
	ChecksPassing      bool
	ChecksObserved     bool
	Approvals          int
	NonAuthorApprovals int
	GateBlocked        bool
	GateReasons        []string
	ActionRequired     string
	ActionReason       string
	Warnings           []string
}

type ghPRSummary struct {
	Number         int    `json:"number"`
	URL            string `json:"url"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	HeadRefOID     string `json:"headRefOid"`
	BaseRefName    string `json:"baseRefName"`
	ReviewDecision string `json:"reviewDecision"`
	MergedAt       string `json:"mergedAt"`
	MergeCommit    *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	LatestReviews []struct {
		State  string `json:"state"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
	} `json:"latestReviews"`
	StatusCheckRollup []struct {
		Conclusion string `json:"conclusion"`
		Status     string `json:"status"`
	} `json:"statusCheckRollup"`
}

type PRApprovalRequiredError struct {
	CRID   int
	Branch string
	Reason string
}

type PRActionRequiredError struct {
	Cause            error
	Sentinel         error
	Summary          string
	Reason           string
	ActionName       string
	SuggestedCommand string
	Context          map[string]any
}

func (e *PRActionRequiredError) Error() string {
	if e == nil {
		return "pr action required"
	}
	summary := strings.TrimSpace(e.Summary)
	if summary == "" {
		summary = "pr action required"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		return summary
	}
	return fmt.Sprintf("%s: %s", summary, reason)
}

func (e *PRActionRequiredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *PRActionRequiredError) Is(target error) bool {
	if e == nil {
		return false
	}
	return e.Sentinel != nil && target == e.Sentinel
}

func (e *PRActionRequiredError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	context := map[string]any{}
	for k, v := range e.Context {
		context[k] = v
	}
	details := map[string]any{
		"reason": nonEmptyTrimmed(e.Reason, "manual follow-up required"),
		"action_required": map[string]any{
			"type":   "manual",
			"name":   nonEmptyTrimmed(e.ActionName, "manual_follow_up"),
			"reason": nonEmptyTrimmed(e.Reason, "manual follow-up required"),
		},
	}
	if strings.TrimSpace(e.SuggestedCommand) != "" {
		details["suggested_command"] = strings.TrimSpace(e.SuggestedCommand)
		actionRequired := details["action_required"].(map[string]any)
		actionRequired["suggested_command"] = strings.TrimSpace(e.SuggestedCommand)
	}
	if len(context) > 0 {
		details["context"] = context
	}
	return details
}

func (e *PRApprovalRequiredError) Error() string {
	if e == nil {
		return ErrPRApprovalRequired.Error()
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "approve PR create/open to proceed"
	}
	return fmt.Sprintf("%s: %s", ErrPRApprovalRequired, reason)
}

func (e *PRApprovalRequiredError) Is(target error) bool {
	return target == ErrPRApprovalRequired
}

func (e *PRApprovalRequiredError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"cr_id":        e.CRID,
		"branch":       strings.TrimSpace(e.Branch),
		"action":       "open_pr",
		"reason":       nonEmptyTrimmed(e.Reason, "approve PR create/open to proceed"),
		"approve_flag": "--approve-pr-open",
		"action_required": map[string]any{
			"type": "agent_approval",
			"name": "open_pr",
		},
	}
}

func classifyGHCommandError(err error, args []string) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "gh auth login") ||
		strings.Contains(lower, "not logged into") ||
		strings.Contains(lower, "authentication required") ||
		strings.Contains(lower, "token has expired") {
		return &PRActionRequiredError{
			Cause:            err,
			Sentinel:         ErrGHAuthRequired,
			Summary:          "GitHub authentication required",
			Reason:           "gh is not authenticated for this repository",
			ActionName:       "gh_auth_login",
			SuggestedCommand: "gh auth login",
			Context:          map[string]any{"command": "gh " + strings.Join(args, " ")},
		}
	}
	if strings.Contains(lower, "resource not accessible by integration") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "insufficient permission") ||
		strings.Contains(lower, "must have write access") ||
		strings.Contains(lower, "not permitted") {
		return &PRActionRequiredError{
			Cause:      err,
			Sentinel:   ErrPRPermissionDenied,
			Summary:    "Permission denied for PR operation",
			Reason:     "current GitHub identity cannot complete this PR action",
			ActionName: "request_reviewer_merge",
			Context: map[string]any{
				"command": "gh " + strings.Join(args, " "),
			},
		}
	}
	return err
}

func classifyPushCommandError(err error, branch string) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "permission to") && strings.Contains(lower, "denied") ||
		strings.Contains(lower, "write access to repository not granted") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "403 forbidden") ||
		strings.Contains(lower, "the requested url returned error: 403") {
		return &PRActionRequiredError{
			Cause:            err,
			Sentinel:         ErrPushPermissionDenied,
			Summary:          "Push denied for CR branch",
			Reason:           "origin rejected branch push",
			ActionName:       "request_push_access",
			SuggestedCommand: fmt.Sprintf("git push -u origin %s", strings.TrimSpace(branch)),
			Context:          map[string]any{"branch": strings.TrimSpace(branch), "remote": "origin"},
		}
	}
	return err
}

func (s *Service) PRContext(id int) (*PRContextView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil, fmt.Errorf("pr context is only available when merge.mode=pr_gate")
	}
	return s.buildPRContextView(cr)
}

func (s *Service) PRDraft(id int) (*PRContextView, error) {
	return s.PRContext(id)
}

func (s *Service) PROpen(id int, approve bool) (*PRStatusView, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil, fmt.Errorf("cr pr open is only available when merge.mode=pr_gate")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.openOrSyncPRForCR(cr, policy, approve); err != nil {
		return nil, err
	}
	return s.PRStatus(id)
}

func (s *Service) PRSync(id int) (*PRStatusView, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil, fmt.Errorf("cr pr sync is only available when merge.mode=pr_gate")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.PR.Number <= 0 {
		return nil, fmt.Errorf("cr %d has no linked PR; run `sophia cr pr open %d`", id, id)
	}
	if _, err := s.openOrSyncPRForCR(cr, policy, true); err != nil {
		return nil, err
	}
	return s.PRStatus(id)
}

func (s *Service) PRReady(id int) (*PRStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.PR.Number <= 0 {
		return nil, fmt.Errorf("cr %d has no linked PR", id)
	}
	if _, err := s.runCommand("gh", "pr", "ready", strconv.Itoa(cr.PR.Number)); err != nil {
		return nil, err
	}
	now := s.timestamp()
	actor := s.git.Actor()
	_ = s.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRPRReady,
		Summary: fmt.Sprintf("Marked PR #%d ready for review", cr.PR.Number),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	return s.PRStatus(id)
}

func (s *Service) PRStatus(id int) (*PRStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.PR.Number <= 0 {
		return &PRStatusView{
			CRID:           cr.ID,
			CRUID:          strings.TrimSpace(cr.UID),
			ActionRequired: "open_pr",
			ActionReason:   "no linked PR",
			GateBlocked:    true,
			GateReasons:    []string{"no linked PR"},
		}, nil
	}
	status, err := s.fetchGHPRStatus(cr)
	if err != nil {
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	status.GateBlocked, status.GateReasons = evaluatePRGate(policy, status)
	if status.Merged {
		if reconcileErr := s.reconcileRemoteMergedPR(cr, status); reconcileErr != nil {
			status.Warnings = append(status.Warnings, fmt.Sprintf("remote merge reconciliation failed: %v", reconcileErr))
		}
	}
	return status, nil
}

func evaluatePRGate(policy *model.RepoPolicy, status *PRStatusView) (bool, []string) {
	reasons := []string{}
	if policy == nil || status == nil {
		return false, reasons
	}
	requiredApprovals := defaultMergeRequiredApprovals
	if policy.Merge.RequiredApprovals != nil {
		requiredApprovals = *policy.Merge.RequiredApprovals
	}
	requireNonAuthor := defaultMergeRequireNonAuthorApproval
	if policy.Merge.RequireNonAuthorApproval != nil {
		requireNonAuthor = *policy.Merge.RequireNonAuthorApproval
	}
	requireReady := defaultMergeRequireReadyForReview
	if policy.Merge.RequireReadyForReview != nil {
		requireReady = *policy.Merge.RequireReadyForReview
	}
	requireChecks := defaultMergeRequirePassingChecks
	if policy.Merge.RequirePassingChecks != nil {
		requireChecks = *policy.Merge.RequirePassingChecks
	}
	if requireReady && status.Draft {
		reasons = append(reasons, "PR is still draft")
	}
	if requiredApprovals > 0 && status.Approvals < requiredApprovals {
		reasons = append(reasons, fmt.Sprintf("insufficient approvals (%d/%d)", status.Approvals, requiredApprovals))
	}
	if requireNonAuthor && status.NonAuthorApprovals < 1 {
		reasons = append(reasons, "missing non-author approval")
	}
	if requireChecks && !status.ChecksObserved {
		reasons = append(reasons, "required checks are not reported")
	} else if requireChecks && !status.ChecksPassing {
		reasons = append(reasons, "required checks are not passing")
	}
	sort.Strings(reasons)
	return len(reasons) > 0, reasons
}

func (s *Service) mergePRGateCRUnlocked(id int, opts MergeCROptions, policy *model.RepoPolicy) (*MergeCRResult, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if _, err := s.openOrSyncPRForCR(cr, policy, opts.ApprovePROpen); err != nil {
		return nil, err
	}
	status, err := s.fetchGHPRStatus(cr)
	if err != nil {
		return nil, err
	}
	blocked, reasons := evaluatePRGate(policy, status)
	result := &MergeCRResult{
		MergedCommit: "",
		Warnings:     []string{},
		MergeMode:    "pr_gate",
		PRURL:        status.URL,
		Action:       "pr_published",
		GateBlocked:  blocked,
		GateReasons:  reasons,
	}
	if blocked {
		result.ActionReason = strings.Join(reasons, "; ")
	}
	return result, nil
}

func (s *Service) mergePRGateFinalizeUnlocked(id int, opts MergeCROptions, policy *model.RepoPolicy) (*MergeCRResult, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.PR.Number <= 0 {
		return nil, fmt.Errorf("cr %d has no linked PR", id)
	}
	status, err := s.fetchGHPRStatus(cr)
	if err != nil {
		return nil, err
	}
	blocked, reasons := evaluatePRGate(policy, status)
	if blocked && strings.TrimSpace(opts.OverrideReason) == "" {
		return &MergeCRResult{
			MergeMode:    "pr_gate",
			PRURL:        status.URL,
			Action:       "awaiting_reviews",
			ActionReason: strings.Join(reasons, "; "),
			GateBlocked:  true,
			GateReasons:  reasons,
		}, fmt.Errorf("merge blocked: %s", strings.Join(reasons, "; "))
	}
	if status.Merged {
		if err := s.reconcileRemoteMergedPR(cr, status); err != nil {
			return nil, err
		}
		return &MergeCRResult{MergedCommit: strings.TrimSpace(status.MergedCommit), MergeMode: "pr_gate", PRURL: status.URL, Action: "already_merged_remote"}, nil
	}
	if _, err := s.runCommand("gh", buildPRMergeArgs(cr.PR.Number, !opts.KeepBranch)...); err != nil {
		return nil, err
	}
	status, err = s.fetchGHPRStatus(cr)
	if err != nil {
		return nil, err
	}
	if err := s.reconcileRemoteMergedPR(cr, status); err != nil {
		return nil, err
	}
	return &MergeCRResult{
		MergedCommit: strings.TrimSpace(status.MergedCommit),
		MergeMode:    "pr_gate",
		PRURL:        status.URL,
		Action:       "merged_remote",
		Warnings:     []string{},
	}, nil
}

func (s *Service) buildPRContextView(cr *model.CR) (*PRContextView, error) {
	review, err := s.ReviewCR(cr.ID)
	if err != nil {
		return nil, err
	}
	md := renderManagedPRBlock(review)
	hash := hashString(md)
	view := &PRContextView{
		CRID:     cr.ID,
		CRUID:    strings.TrimSpace(cr.UID),
		Title:    cr.Title,
		PRTitle:  strings.TrimSpace(cr.Title),
		Branch:   strings.TrimSpace(cr.Branch),
		BaseRef:  strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)),
		Markdown: md,
		BodyHash: hash,
		Warnings: []string{},
	}
	return view, nil
}

func renderManagedPRBlock(review *Review) string {
	if review == nil || review.CR == nil {
		return "<!-- sophia:managed:start -->\n(no content)\n<!-- sophia:managed:end -->\n"
	}
	cr := review.CR
	var b strings.Builder
	b.WriteString("<!-- sophia:managed:start -->\n")
	b.WriteString("## Sophia CR Context\n\n")
	b.WriteString("### Identity\n")
	b.WriteString(fmt.Sprintf("- CR Title: %s\n", strings.TrimSpace(cr.Title)))
	b.WriteString(fmt.Sprintf("- Branch: %s\n", strings.TrimSpace(cr.Branch)))
	b.WriteString(fmt.Sprintf("- Base Ref: %s\n", strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))))
	if review.Impact != nil {
		b.WriteString(fmt.Sprintf("- Risk Tier: %s\n", strings.TrimSpace(review.Impact.RiskTier)))
	}
	b.WriteString("\n### Contract Why\n")
	if strings.TrimSpace(cr.Contract.Why) == "" {
		b.WriteString("- WARNING: missing contract.why\n")
	} else {
		b.WriteString(strings.TrimSpace(cr.Contract.Why) + "\n")
	}
	b.WriteString("\n### Scope\n")
	for _, scope := range cr.Contract.Scope {
		b.WriteString("- " + scope + "\n")
	}
	b.WriteString("\n### Invariants\n")
	for _, item := range cr.Contract.Invariants {
		b.WriteString("- " + item + "\n")
	}
	b.WriteString("\n### Non Goals\n")
	for _, item := range cr.Contract.NonGoals {
		b.WriteString("- " + item + "\n")
	}
	b.WriteString("\n### Tasks\n")
	for _, task := range cr.Subtasks {
		state := "[ ]"
		if task.Status == model.TaskStatusDone {
			state = "[x]"
		}
		drift := ""
		if task.CheckpointOrphan {
			drift = " (scope-drift)"
		}
		commit := strings.TrimSpace(task.CheckpointCommit)
		if commit == "" {
			commit = "-"
		}
		if len(commit) > 12 {
			commit = commit[:12]
		}
		b.WriteString(fmt.Sprintf("- %s %s | checkpoint: %s%s\n", state, strings.TrimSpace(task.Title), commit, drift))
	}
	b.WriteString("\n### Evidence\n")
	if len(cr.Evidence) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, ev := range cr.Evidence {
			exitCode := ""
			if ev.ExitCode != nil {
				exitCode = fmt.Sprintf(" (exit=%d)", *ev.ExitCode)
			}
			b.WriteString(fmt.Sprintf("- %s: %s%s\n", strings.TrimSpace(ev.Type), strings.TrimSpace(ev.Summary), exitCode))
		}
	}
	b.WriteString("\n### Diff Summary\n")
	b.WriteString(nonEmptyTrimmed(strings.TrimSpace(review.ShortStat), "(missing)") + "\n")
	b.WriteString("\n### Rollback Plan\n")
	if strings.TrimSpace(cr.Contract.RollbackPlan) == "" {
		b.WriteString("- WARNING: missing rollback_plan\n")
	} else {
		b.WriteString(strings.TrimSpace(cr.Contract.RollbackPlan) + "\n")
	}
	b.WriteString("\n<!-- sophia:managed:end -->\n")
	return b.String()
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) openOrSyncPRForCR(cr *model.CR, policy *model.RepoPolicy, approve bool) (*ghPRSummary, error) {
	if cr == nil {
		return nil, fmt.Errorf("cr is required")
	}
	hadLinkedPR := cr.PR.Number > 0
	if err := s.stageArchiveForPRGate(cr, policy); err != nil {
		return nil, err
	}
	if err := s.pushBranchIfNeeded(cr); err != nil {
		return nil, err
	}
	ctx, err := s.buildPRContextView(cr)
	if err != nil {
		return nil, err
	}
	repo, err := s.currentRemoteRepo("origin")
	if err != nil {
		return nil, err
	}
	pr, err := s.findPRByHead(cr.Branch)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		if !approve {
			cr.PR.AwaitingOpenApproval = true
			cr.PR.AwaitingOpenApprovalNote = "approve PR creation/open to proceed"
			cr.UpdatedAt = s.timestamp()
			if saveErr := s.store.SaveCR(cr); saveErr != nil {
				return nil, saveErr
			}
			return nil, &PRApprovalRequiredError{
				CRID:   cr.ID,
				Branch: strings.TrimSpace(cr.Branch),
				Reason: "approve PR create/open to proceed",
			}
		}
		url, createErr := s.createDraftPR(cr, ctx.Markdown)
		if createErr != nil {
			return nil, createErr
		}
		pr = &ghPRSummary{URL: strings.TrimSpace(url)}
		if refreshed, refreshErr := s.findPRByHead(cr.Branch); refreshErr == nil && refreshed != nil {
			pr = refreshed
		}
	}
	if pr != nil && pr.Number <= 0 {
		if parsed := parsePRNumberFromURL(pr.URL); parsed > 0 {
			if byNumber, byNumberErr := s.fetchPRByNumber(parsed); byNumberErr == nil && byNumber != nil {
				pr = byNumber
			} else {
				pr.Number = parsed
			}
		}
	}
	if pr == nil || pr.Number <= 0 {
		return nil, fmt.Errorf("unable to resolve PR number for branch %q after create/sync", strings.TrimSpace(cr.Branch))
	}
	if strings.TrimSpace(pr.URL) == "" {
		if byNumber, byNumberErr := s.fetchPRByNumber(pr.Number); byNumberErr == nil && byNumber != nil {
			pr = byNumber
		}
	}

	finalBody, bodyErr := s.patchManagedBody(pr, ctx.Markdown)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if pr.Number > 0 {
		if err := s.editPR(pr.Number, cr.Title, finalBody); err != nil {
			return nil, err
		}
	}
	if pr.Number > 0 {
		if commentErr := s.syncCheckpointComments(pr.Number, cr); commentErr != nil {
			return nil, commentErr
		}
	}
	now := s.timestamp()
	cr.PR.Provider = prProviderGitHub
	cr.PR.Repo = repo
	cr.PR.Number = pr.Number
	cr.PR.URL = strings.TrimSpace(pr.URL)
	cr.PR.State = strings.TrimSpace(pr.State)
	cr.PR.Draft = pr.IsDraft
	cr.PR.LastHeadSHA = strings.TrimSpace(pr.HeadRefOID)
	cr.PR.LastBaseRef = strings.TrimSpace(pr.BaseRefName)
	cr.PR.LastBodyHash = hashString(finalBody)
	cr.PR.LastSyncedAt = now
	cr.PR.AwaitingOpenApproval = false
	cr.PR.AwaitingOpenApprovalNote = ""
	cr.UpdatedAt = now
	if err := s.store.SaveCR(cr); err != nil {
		return nil, err
	}
	if !hadLinkedPR {
		_ = s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   s.git.Actor(),
			Type:    model.EventTypeCRPROpened,
			Summary: fmt.Sprintf("Opened PR #%d", pr.Number),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
		})
	}
	_ = s.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   s.git.Actor(),
		Type:    model.EventTypeCRPRSynced,
		Summary: fmt.Sprintf("Synced PR #%d context", pr.Number),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	return pr, nil
}

func (s *Service) patchManagedBody(pr *ghPRSummary, managed string) (string, error) {
	if pr == nil || pr.Number <= 0 {
		return managed, nil
	}
	body, err := s.readPRBody(pr.Number)
	if err != nil {
		return "", fmt.Errorf("read PR body: %w", err)
	}
	return mergeManagedPRBody(body, managed)
}

func mergeManagedPRBody(existingBody, managed string) (string, error) {
	start := "<!-- sophia:managed:start -->"
	end := "<!-- sophia:managed:end -->"
	existing := strings.TrimSpace(existingBody)
	managed = strings.TrimSpace(managed)
	if managed == "" {
		return existingBody, nil
	}
	si := strings.Index(existing, start)
	ei := strings.Index(existing, end)
	hasStart := si >= 0
	hasEnd := ei >= 0
	if hasStart != hasEnd {
		return "", fmt.Errorf("managed marker corruption detected in PR body")
	}
	if !hasStart {
		if existing == "" {
			return managed + "\n", nil
		}
		return strings.TrimRight(existingBody, "\n") + "\n\n" + managed + "\n", nil
	}
	ei += len(end)
	prefix := strings.TrimRight(existing[:si], "\n")
	suffix := strings.TrimLeft(existing[ei:], "\n")
	var out strings.Builder
	if strings.TrimSpace(prefix) != "" {
		out.WriteString(prefix)
		out.WriteString("\n\n")
	}
	out.WriteString(managed)
	if strings.TrimSpace(suffix) != "" {
		out.WriteString("\n\n")
		out.WriteString(suffix)
	}
	return out.String() + "\n", nil
}

func (s *Service) syncCheckpointComments(prNumber int, cr *model.CR) error {
	if cr == nil || prNumber <= 0 {
		return nil
	}
	posted := map[string]struct{}{}
	for _, key := range cr.PR.CheckpointCommentKeys {
		posted[strings.TrimSpace(key)] = struct{}{}
	}
	for _, task := range cr.Subtasks {
		if task.Status != model.TaskStatusDone {
			continue
		}
		commit := strings.TrimSpace(task.CheckpointCommit)
		if commit == "" {
			continue
		}
		key := fmt.Sprintf("task:%d:%s", task.ID, commit)
		if _, exists := posted[key]; exists {
			continue
		}
		short := commit
		if len(short) > 12 {
			short = short[:12]
		}
		drift := "no"
		if task.CheckpointOrphan {
			drift = "yes"
		}
		comment := fmt.Sprintf("Checkpoint synced: %s | task: %s | scope_drift: %s", short, strings.TrimSpace(task.Title), drift)
		if _, err := s.runCommand("gh", "pr", "comment", strconv.Itoa(prNumber), "--body", comment); err != nil {
			return err
		}
		cr.PR.CheckpointCommentKeys = append(cr.PR.CheckpointCommentKeys, key)
		posted[key] = struct{}{}
	}
	sort.Strings(cr.PR.CheckpointCommentKeys)
	cr.PR.CheckpointCommentKeys = dedupeStrings(cr.PR.CheckpointCommentKeys)
	return nil
}

func (s *Service) findPRByHead(branch string) (*ghPRSummary, error) {
	if strings.TrimSpace(branch) == "" {
		return nil, fmt.Errorf("branch is required")
	}
	out, err := s.runCommand("gh", "pr", "list", "--head", branch, "--state", "all", "--json", "number,url,state,isDraft,headRefOid,baseRefName,reviewDecision,mergedAt,mergeCommit")
	if err != nil {
		return nil, err
	}
	var items []ghPRSummary
	if unmarshalErr := json.Unmarshal([]byte(out), &items); unmarshalErr != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", unmarshalErr)
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func (s *Service) fetchPRByNumber(number int) (*ghPRSummary, error) {
	if number <= 0 {
		return nil, fmt.Errorf("pr number is required")
	}
	out, err := s.runCommand("gh", "pr", "view", strconv.Itoa(number), "--json", "number,url,state,isDraft,headRefOid,baseRefName,reviewDecision,mergedAt,mergeCommit")
	if err != nil {
		return nil, err
	}
	var pr ghPRSummary
	if unmarshalErr := json.Unmarshal([]byte(out), &pr); unmarshalErr != nil {
		return nil, fmt.Errorf("parse gh pr view output: %w", unmarshalErr)
	}
	if pr.Number <= 0 {
		pr.Number = number
	}
	return &pr, nil
}

func (s *Service) createDraftPR(cr *model.CR, body string) (string, error) {
	if cr == nil {
		return "", fmt.Errorf("cr is required")
	}
	base := strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	if base == "" {
		base = "main"
	}
	bodyFile, err := writeTempBody(body)
	if err != nil {
		return "", err
	}
	defer os.Remove(bodyFile)
	out, err := s.runCommand("gh", "pr", "create", "--draft", "--title", strings.TrimSpace(cr.Title), "--body-file", bodyFile, "--base", base, "--head", strings.TrimSpace(cr.Branch))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func writeTempBody(body string) (string, error) {
	f, err := os.CreateTemp("", "sophia-pr-body-*.md")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(body); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func (s *Service) editPR(number int, title, body string) error {
	if number <= 0 {
		return fmt.Errorf("pr number is required")
	}
	bodyFile, err := writeTempBody(body)
	if err != nil {
		return err
	}
	defer os.Remove(bodyFile)
	_, err = s.runCommand("gh", "pr", "edit", strconv.Itoa(number), "--title", strings.TrimSpace(title), "--body-file", bodyFile)
	return err
}

func (s *Service) readPRBody(number int) (string, error) {
	out, err := s.runCommand("gh", "pr", "view", strconv.Itoa(number), "--json", "body")
	if err != nil {
		return "", err
	}
	var payload struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return "", err
	}
	return payload.Body, nil
}

func (s *Service) fetchGHPRStatus(cr *model.CR) (*PRStatusView, error) {
	if cr == nil || cr.PR.Number <= 0 {
		return nil, fmt.Errorf("linked PR is required")
	}
	out, err := s.runCommand("gh", "pr", "view", strconv.Itoa(cr.PR.Number), "--json", "number,url,state,isDraft,reviewDecision,mergedAt,mergeCommit,author,latestReviews,statusCheckRollup,headRefOid,baseRefName")
	if err != nil {
		return nil, err
	}
	var pr ghPRSummary
	if unmarshalErr := json.Unmarshal([]byte(out), &pr); unmarshalErr != nil {
		return nil, fmt.Errorf("parse gh pr view output: %w", unmarshalErr)
	}
	approvals := 0
	nonAuthor := 0
	authorLogin := ""
	if pr.Author != nil {
		authorLogin = strings.TrimSpace(pr.Author.Login)
	}
	for _, review := range pr.LatestReviews {
		if strings.EqualFold(strings.TrimSpace(review.State), "APPROVED") {
			approvals++
			if !strings.EqualFold(strings.TrimSpace(review.Author.Login), authorLogin) {
				nonAuthor++
			}
		}
	}
	checksObserved := len(pr.StatusCheckRollup) > 0
	checksPassing := checksObserved
	if checksObserved {
		for _, check := range pr.StatusCheckRollup {
			status := strings.ToUpper(strings.TrimSpace(check.Status))
			conclusion := strings.ToUpper(strings.TrimSpace(check.Conclusion))
			if status == "IN_PROGRESS" || status == "QUEUED" {
				checksPassing = false
				break
			}
			switch conclusion {
			case "", "SUCCESS", "NEUTRAL", "SKIPPED":
			default:
				checksPassing = false
			}
		}
	}
	mergedCommit := ""
	if pr.MergeCommit != nil {
		mergedCommit = strings.TrimSpace(pr.MergeCommit.OID)
	}
	if mergedCommit == "" {
		mergedCommit = strings.TrimSpace(pr.HeadRefOID)
	}
	stateUpper := strings.ToUpper(strings.TrimSpace(pr.State))
	merged := stateUpper == "MERGED" || strings.TrimSpace(pr.MergedAt) != ""
	return &PRStatusView{
		CRID:               cr.ID,
		CRUID:              strings.TrimSpace(cr.UID),
		Provider:           prProviderGitHub,
		Repo:               strings.TrimSpace(cr.PR.Repo),
		Number:             pr.Number,
		URL:                strings.TrimSpace(pr.URL),
		State:              strings.TrimSpace(pr.State),
		Draft:              pr.IsDraft,
		ReviewDecision:     strings.TrimSpace(pr.ReviewDecision),
		Merged:             merged,
		MergedAt:           strings.TrimSpace(pr.MergedAt),
		MergedCommit:       mergedCommit,
		ChecksPassing:      checksPassing,
		ChecksObserved:     checksObserved,
		Approvals:          approvals,
		NonAuthorApprovals: nonAuthor,
		GateReasons:        []string{},
		Warnings:           []string{},
	}, nil
}

func (s *Service) reconcileRemoteMergedPR(cr *model.CR, status *PRStatusView) error {
	if cr == nil || status == nil || !status.Merged {
		return nil
	}
	if cr.Status == model.StatusMerged && strings.TrimSpace(cr.MergedCommit) != "" {
		return nil
	}
	now := s.timestamp()
	actor := s.git.Actor()
	mergedAt := strings.TrimSpace(status.MergedAt)
	if mergedAt == "" {
		mergedAt = now
	}
	mergedCommit := strings.TrimSpace(status.MergedCommit)
	if mergedCommit == "" {
		return fmt.Errorf("merged PR is missing merged commit")
	}
	cr.Status = model.StatusMerged
	cr.MergedAt = mergedAt
	cr.MergedBy = actor
	cr.MergedCommit = mergedCommit
	cr.PR.State = "MERGED"
	cr.PR.Draft = false
	cr.PR.LastMergedAt = mergedAt
	cr.PR.LastMergedCommit = mergedCommit
	cr.PR.LastStatusCheckedAt = now
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRPRMergedRemote,
		Summary: fmt.Sprintf("Detected remote PR merge for CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	cr.Events = append(cr.Events, model.Event{
		TS:      now,
		Actor:   actor,
		Type:    model.EventTypeCRMerged,
		Summary: fmt.Sprintf("Merged CR %d", cr.ID),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	if err := s.store.SaveCR(cr); err != nil {
		return err
	}
	return s.syncCRRef(cr)
}

func (s *Service) pushBranchIfNeeded(cr *model.CR) error {
	if cr == nil {
		return fmt.Errorf("cr is required")
	}
	branch := strings.TrimSpace(cr.Branch)
	if branch == "" {
		return fmt.Errorf("cr branch is empty")
	}
	if _, err := s.runCommand("git", "ls-remote", "--exit-code", "--heads", "origin", branch); err == nil {
		return nil
	}
	_, err := s.runCommand("git", "push", "-u", "origin", branch)
	return classifyPushCommandError(err, branch)
}

func (s *Service) stageArchiveForPRGate(cr *model.CR, policy *model.RepoPolicy) error {
	if cr == nil || policy == nil {
		return nil
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil
	}
	if !archivePolicyEnabled(policy.Archive) {
		return nil
	}
	if err := s.requireArchiveConfigSupported(policy.Archive); err != nil {
		return err
	}
	mergeGit, owner, err := s.gitClientForBranch(cr.Branch)
	if err != nil {
		return err
	}
	workDir := strings.TrimSpace(s.git.WorkDir)
	if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
		workDir = strings.TrimSpace(owner.Path)
	}
	if workDir == "" {
		return fmt.Errorf("unable to determine worktree path for CR archive staging")
	}
	if dirty, summary, dirtyErr := s.workingTreeDirtySummaryFor(mergeGit); dirtyErr != nil {
		return dirtyErr
	} else if dirty {
		return fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}
	currentBranch, err := mergeGit.CurrentBranch()
	if err != nil {
		return err
	}
	currentBranch = strings.TrimSpace(currentBranch)
	branch := strings.TrimSpace(cr.Branch)
	switched := false
	if currentBranch != branch {
		if owner != nil && !s.isCurrentWorktreePath(owner.Path) {
			return fmt.Errorf("%w: branch %q is checked out in worktree %q", ErrBranchInOtherWorktree, branch, owner.Path)
		}
		if err := mergeGit.CheckoutBranch(branch); err != nil {
			return err
		}
		switched = true
	}
	if switched {
		defer func() {
			_ = mergeGit.CheckoutBranch(currentBranch)
		}()
	}
	archivePath := archiveRevisionPath(filepath.Join(workDir, policy.Archive.Path), cr.ID, 1)
	if exists, existsErr := archiveFileExists(archivePath); existsErr != nil {
		return existsErr
	} else if exists {
		return nil
	}
	baseRef := strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	if baseRef == "" {
		baseRef = "main"
	}
	baseParent, err := mergeGit.ResolveRef(baseRef)
	if err != nil {
		return err
	}
	crParent, err := mergeGit.ResolveRef(branch)
	if err != nil {
		return err
	}
	gitSummary, err := buildArchiveGitSummaryFromCachedDiff(mergeGit, baseParent, crParent)
	if err != nil {
		return err
	}
	archiveCR := *cr
	archiveCR.Status = model.StatusMerged
	archiveCR.MergedAt = s.timestamp()
	archiveCR.MergedBy = mergeGit.Actor()
	archive := buildCRArchiveDocument(&archiveCR, 1, "", archiveCR.MergedAt, gitSummary)
	payload, err := marshalCRArchiveYAML(archive)
	if err != nil {
		return err
	}
	if err := writeArchivePayload(archivePath, payload); err != nil {
		return err
	}
	relPath, err := relativeToRootPath(workDir, archivePath)
	if err != nil {
		return err
	}
	if err := mergeGit.StagePaths([]string{relPath}); err != nil {
		return err
	}
	commitMsg := fmt.Sprintf("chore: stage CR %d archive artifact for PR review", cr.ID)
	if err := mergeGit.Commit(commitMsg); err != nil {
		return err
	}
	return nil
}

func (s *Service) currentRemoteRepo(remote string) (string, error) {
	if strings.TrimSpace(remote) == "" {
		remote = "origin"
	}
	url, err := s.runCommand("git", "config", "--get", fmt.Sprintf("remote.%s.url", remote))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(url), nil
}

func (s *Service) runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = s.git.WorkDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		commandErr := fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		if strings.EqualFold(strings.TrimSpace(name), "gh") {
			return "", classifyGHCommandError(commandErr, args)
		}
		if strings.EqualFold(strings.TrimSpace(name), "git") && len(args) > 0 && strings.EqualFold(strings.TrimSpace(args[0]), "push") {
			branch := ""
			if len(args) > 0 {
				branch = strings.TrimSpace(args[len(args)-1])
			}
			return "", classifyPushCommandError(commandErr, branch)
		}
		return "", commandErr
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (s *Service) ensurePRGateReconciledInStatus(id int) {
	_ = s.withMutationLock(func() error {
		cr, err := s.store.LoadCR(id)
		if err != nil {
			return err
		}
		if cr.Status == model.StatusMerged || cr.PR.Number <= 0 {
			return nil
		}
		status, statusErr := s.fetchGHPRStatus(cr)
		if statusErr != nil {
			return nil
		}
		if status.Merged {
			if recErr := s.reconcileRemoteMergedPR(cr, status); recErr != nil {
				return nil
			}
		}
		return nil
	})
}

func (s *Service) mergePRApprovalRequiredResult(cr *model.CR) *MergeCRResult {
	url := ""
	if cr != nil {
		url = strings.TrimSpace(cr.PR.URL)
	}
	return &MergeCRResult{
		MergeMode:    "pr_gate",
		PRURL:        url,
		Action:       "approval_required",
		ActionReason: "approve PR create/open to proceed",
		GateBlocked:  true,
		GateReasons:  []string{"approval required for PR open/create"},
	}
}

func parsePRNumberFromURL(url string) int {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return 0
	}
	parts := strings.Split(strings.TrimRight(trimmed, "/"), "/")
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func buildPRMergeArgs(number int, deleteBranch bool) []string {
	args := []string{"pr", "merge", strconv.Itoa(number), "--merge"}
	if deleteBranch {
		args = append(args, "--delete-branch")
	}
	return args
}
