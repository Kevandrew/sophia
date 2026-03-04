package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sophia/internal/model"
	"sort"
	"strconv"
	"strings"
)

const (
	prProviderGitHub     = "github"
	prLinkageHealthy     = "healthy"
	prLinkageNoLinkedPR  = "no_linked_pr"
	prLinkagePRNotFound  = "pr_not_found"
	prLinkageClosed      = "closed_unmerged"
	prLinkageMismatch    = "linkage_mismatch"
	prActionOpenPR       = "open_pr"
	prActionReadyPR      = "ready_pr"
	prActionReadyBlocked = "ready_pr_blocked"
	prActionUnreadyPR    = "unready_pr"
	prActionClosePR      = "close_pr"
	prActionReconcilePR  = "reconcile_pr"
	prActionReopenPR     = "reopen_pr"
	prActionCreatePR     = "create_pr"
	prGateNoLinkedReason = "no linked PR"
	prGateNotFoundReason = "linked PR not found"
	prGateClosedReason   = "linked PR is closed without merge"
	prGateMismatchReason = "linked PR linkage mismatch"

	prReconcileModeRelink = "relink"
	prReconcileModeReopen = "reopen"
	prReconcileModeCreate = "create"

	prReadyBlockedReasonNoCheckpoints = "pre_implementation_no_checkpoints"
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
	HeadRefOID         string
	HeadRefName        string
	BaseRefName        string
	Approvals          int
	NonAuthorApprovals int
	GateBlocked        bool
	GateReasons        []string
	LinkageState       string
	ActionRequired     string
	ActionReason       string
	SuggestedCommands  []string
	Warnings           []string
}

type PRReconcileView struct {
	CRID              int
	CRUID             string
	Mode              string
	Mutated           bool
	Action            string
	ActionReason      string
	BeforePRNumber    int
	AfterPRNumber     int
	BeforeLinkage     string
	AfterLinkage      string
	SuggestedCommands []string
	Warnings          []string
}

type ghPRSummary struct {
	Number         int    `json:"number"`
	URL            string `json:"url"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	HeadRefOID     string `json:"headRefOid"`
	HeadRefName    string `json:"headRefName"`
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
		State      string `json:"state"`
	} `json:"statusCheckRollup"`
	UpdatedAt string `json:"updatedAt"`
}

type PRApprovalRequiredError struct {
	CRID   int
	Branch string
	Reason string
}

type PRReadyBlockedError struct {
	CRID              int
	ReasonCode        string
	Reason            string
	SuggestedCommands []string
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
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil, fmt.Errorf("cr pr ready is only available when merge.mode=pr_gate")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.PR.Number <= 0 {
		return nil, fmt.Errorf("cr %d has no linked PR", id)
	}
	if !hasImplementationCheckpointProgress(cr) {
		return nil, &PRReadyBlockedError{
			CRID:              cr.ID,
			ReasonCode:        prReadyBlockedReasonNoCheckpoints,
			Reason:            "CR has no task checkpoint commits yet; keep PR draft until implementation checkpoints exist.",
			SuggestedCommands: prReadyBlockedSuggestedCommands(cr.ID),
		}
	}
	if _, err := s.runGH(s.ghRepoSelectorForCR(cr), "pr", "ready", strconv.Itoa(cr.PR.Number)); err != nil {
		return nil, err
	}
	now := s.timestamp()
	repoSelector := s.ghRepoSelectorForCR(cr)
	if refreshed, refreshErr := s.fetchPRByNumber(repoSelector, cr.PR.Number); refreshErr == nil && refreshed != nil {
		applyPRSummaryToCRLink(cr, refreshed, nonEmptyTrimmed(repoSelector, cr.PR.Repo), now)
		if saveErr := s.store.SaveCR(cr); saveErr != nil {
			return nil, saveErr
		}
	}
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

func (s *Service) PRUnready(id int) (*PRStatusView, error) {
	return s.runPRLifecycleTransition(id, []string{"pr", "ready", "<pr-number>", "--undo"}, model.EventTypeCRReconciled, "Moved PR #%d back to draft", "cr pr unready")
}

func (s *Service) PRClose(id int) (*PRStatusView, error) {
	return s.runPRLifecycleTransition(id, []string{"pr", "close", "<pr-number>"}, model.EventTypeCRReconciled, "Closed PR #%d without merge", "cr pr close")
}

func (s *Service) PRReopen(id int) (*PRStatusView, error) {
	return s.runPRLifecycleTransition(id, []string{"pr", "reopen", "<pr-number>"}, model.EventTypeCRReconciled, "Reopened PR #%d", "cr pr reopen")
}

func (s *Service) runPRLifecycleTransition(id int, args []string, eventType string, summaryFmt string, commandName string) (*PRStatusView, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil, fmt.Errorf("%s is only available when merge.mode=pr_gate", strings.TrimSpace(commandName))
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.PR.Number <= 0 {
		return nil, fmt.Errorf("cr %d has no linked PR", id)
	}
	runArgs := append([]string(nil), args...)
	// Command templates pass the CR id placeholder; replace with linked PR number.
	for i := range runArgs {
		if strings.TrimSpace(runArgs[i]) == "<pr-number>" {
			runArgs[i] = strconv.Itoa(cr.PR.Number)
		}
	}
	repoSelector := s.ghRepoSelectorForCR(cr)
	if _, err := s.runGH(repoSelector, runArgs...); err != nil {
		return nil, err
	}
	now := s.timestamp()
	if refreshed, refreshErr := s.fetchPRByNumber(repoSelector, cr.PR.Number); refreshErr == nil && refreshed != nil {
		applyPRSummaryToCRLink(cr, refreshed, nonEmptyTrimmed(repoSelector, cr.PR.Repo), now)
		if saveErr := s.store.SaveCR(cr); saveErr != nil {
			return nil, saveErr
		}
	}
	_ = s.appendCRMutationEventAndSave(cr, model.Event{
		TS:      now,
		Actor:   s.git.Actor(),
		Type:    strings.TrimSpace(eventType),
		Summary: fmt.Sprintf(summaryFmt, cr.PR.Number),
		Ref:     fmt.Sprintf("cr:%d", cr.ID),
	})
	return s.PRStatus(id)
}

func normalizePRReconcileMode(mode string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(mode))
	switch value {
	case prReconcileModeRelink, prReconcileModeReopen, prReconcileModeCreate:
		return value, nil
	default:
		return "", fmt.Errorf("invalid reconcile mode %q (expected relink|reopen|create)", strings.TrimSpace(mode))
	}
}

func (s *Service) PRReconcile(id int, mode string) (*PRReconcileView, error) {
	normalizedMode, err := normalizePRReconcileMode(mode)
	if err != nil {
		return nil, err
	}
	var out *PRReconcileView
	if err := s.withMutationLock(func() error {
		var reconcileErr error
		out, reconcileErr = s.prReconcileUnlocked(id, normalizedMode)
		return reconcileErr
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) prReconcileUnlocked(id int, mode string) (*PRReconcileView, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	if policyMergeMode(policy) != "pr_gate" {
		return nil, fmt.Errorf("cr pr reconcile is only available when merge.mode=pr_gate")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	beforeStatus, _ := s.PRStatus(id)
	beforeNumber := cr.PR.Number
	beforeLinkage := prLinkageHealthy
	if beforeStatus != nil {
		beforeLinkage = nonEmptyTrimmed(beforeStatus.LinkageState, prLinkageHealthy)
	}
	now := s.timestamp()
	actor := s.git.Actor()
	action := ""
	actionReason := ""
	mutated := false

	switch mode {
	case prReconcileModeRelink:
		action = "relinked"
		repoSelector := s.ghRepoSelectorForCR(cr)
		matched, matchErr := s.findPRByHead(repoSelector, cr.Branch, nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
		if matchErr != nil {
			return nil, matchErr
		}
		if matched == nil {
			return nil, fmt.Errorf("no matching PR found for branch %q and base %q; run `sophia cr pr reconcile %d --mode create`", strings.TrimSpace(cr.Branch), strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)), cr.ID)
		}
		if matched.Number <= 0 {
			if parsed := parsePRNumberFromURL(matched.URL); parsed > 0 {
				matched.Number = parsed
			}
		}
		if matched.Number <= 0 {
			return nil, fmt.Errorf("unable to resolve PR number for relink candidate on branch %q", strings.TrimSpace(cr.Branch))
		}
		previous := cr.PR
		applyPRSummaryToCRLink(cr, matched, nonEmptyTrimmed(repoSelector, cr.PR.Repo), now)
		mutated = previous.Number != cr.PR.Number ||
			!strings.EqualFold(strings.TrimSpace(previous.Repo), strings.TrimSpace(cr.PR.Repo)) ||
			!strings.EqualFold(strings.TrimSpace(previous.URL), strings.TrimSpace(cr.PR.URL))
		actionReason = fmt.Sprintf("linked CR %d to PR #%d by branch/base match", cr.ID, cr.PR.Number)
		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
	case prReconcileModeReopen:
		action = "reopened"
		if cr.PR.Number <= 0 {
			return nil, fmt.Errorf("cr %d has no linked PR; run `sophia cr pr reconcile %d --mode create`", cr.ID, cr.ID)
		}
		repoSelector := s.ghRepoSelectorForCR(cr)
		if _, err := s.runGH(repoSelector, "pr", "reopen", strconv.Itoa(cr.PR.Number)); err != nil {
			return nil, err
		}
		refreshed, refreshErr := s.fetchPRByNumber(repoSelector, cr.PR.Number)
		if refreshErr != nil {
			return nil, refreshErr
		}
		applyPRSummaryToCRLink(cr, refreshed, nonEmptyTrimmed(repoSelector, cr.PR.Repo), now)
		mutated = true
		actionReason = fmt.Sprintf("reopened PR #%d", cr.PR.Number)
		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
	case prReconcileModeCreate:
		action = "created"
		repoURL, repoErr := s.currentRemoteRepo("origin")
		if repoErr != nil {
			return nil, repoErr
		}
		repoSelector := normalizeGHRepoSelector(repoURL)
		if strings.TrimSpace(repoSelector) == "" {
			repoSelector = strings.TrimSpace(repoURL)
		}
		ctx, ctxErr := s.buildPRContextView(cr)
		if ctxErr != nil {
			return nil, ctxErr
		}
		if err := s.pushBranchIfNeeded(cr); err != nil {
			return nil, err
		}
		url, createErr := s.createDraftPR(repoSelector, cr, ctx.Markdown)
		if createErr != nil {
			return nil, createErr
		}
		pr := &ghPRSummary{URL: strings.TrimSpace(url)}
		if parsed := parsePRNumberFromURL(pr.URL); parsed > 0 {
			if byNumber, byNumberErr := s.fetchPRByNumber(repoSelector, parsed); byNumberErr == nil && byNumber != nil {
				pr = byNumber
			} else {
				pr.Number = parsed
			}
		}
		if pr.Number <= 0 {
			refreshed, refreshErr := s.findPRByHead(repoSelector, cr.Branch, nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
			if refreshErr != nil {
				return nil, refreshErr
			}
			if refreshed != nil {
				pr = refreshed
			}
		}
		if pr.Number <= 0 {
			return nil, fmt.Errorf("created draft PR but failed to resolve PR number for CR %d", cr.ID)
		}
		applyPRSummaryToCRLink(cr, pr, repoSelector, now)
		cr.PR.LastBodyHash = hashString(ctx.Markdown)
		cr.PR.LastSyncedAt = now
		mutated = true
		actionReason = fmt.Sprintf("created draft PR #%d", cr.PR.Number)
		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
	}

	if mutated {
		meta := map[string]string{
			"mode":         strings.TrimSpace(mode),
			"action":       strings.TrimSpace(action),
			"reason":       strings.TrimSpace(actionReason),
			"before_pr":    strconv.Itoa(beforeNumber),
			"after_pr":     strconv.Itoa(cr.PR.Number),
			"before_state": beforeLinkage,
		}
		if appendErr := s.appendCRMutationEventAndSave(cr, model.Event{
			TS:      now,
			Actor:   actor,
			Type:    model.EventTypeCRReconciled,
			Summary: fmt.Sprintf("PR reconcile (%s): %s", mode, nonEmptyTrimmed(actionReason, action)),
			Ref:     fmt.Sprintf("cr:%d", cr.ID),
			Meta:    meta,
		}); appendErr != nil {
			return nil, appendErr
		}
	}

	afterStatus, _ := s.PRStatus(id)
	afterLinkage := prLinkageHealthy
	if afterStatus != nil {
		afterLinkage = nonEmptyTrimmed(afterStatus.LinkageState, prLinkageHealthy)
	}
	suggested := []string{}
	if afterStatus != nil {
		suggested = append([]string(nil), afterStatus.SuggestedCommands...)
	}
	return &PRReconcileView{
		CRID:              cr.ID,
		CRUID:             strings.TrimSpace(cr.UID),
		Mode:              mode,
		Mutated:           mutated,
		Action:            action,
		ActionReason:      strings.TrimSpace(actionReason),
		BeforePRNumber:    beforeNumber,
		AfterPRNumber:     cr.PR.Number,
		BeforeLinkage:     beforeLinkage,
		AfterLinkage:      afterLinkage,
		SuggestedCommands: cleanAndDedupeStrings(suggested),
		Warnings:          []string{},
	}, nil
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
			LinkageState:   prLinkageNoLinkedPR,
			ActionRequired: prActionOpenPR,
			ActionReason:   prGateNoLinkedReason,
			GateBlocked:    true,
			GateReasons:    []string{prGateNoLinkedReason},
			SuggestedCommands: []string{
				prOpenCommand(cr.ID),
			},
		}, nil
	}
	status, err := s.fetchGHPRStatus(cr)
	if err != nil {
		if isPRNotFoundError(err) {
			return &PRStatusView{
				CRID:           cr.ID,
				CRUID:          strings.TrimSpace(cr.UID),
				Provider:       prProviderGitHub,
				Repo:           strings.TrimSpace(cr.PR.Repo),
				Number:         cr.PR.Number,
				LinkageState:   prLinkagePRNotFound,
				ActionRequired: prActionReconcilePR,
				ActionReason:   fmt.Sprintf("linked PR #%d not found", cr.PR.Number),
				GateBlocked:    true,
				GateReasons:    []string{fmt.Sprintf("%s; run `%s`", prGateNotFoundReason, prReconcileCommand(cr.ID, prReconcileModeCreate))},
				SuggestedCommands: []string{
					prReconcileCommand(cr.ID, prReconcileModeRelink),
					prReconcileCommand(cr.ID, prReconcileModeCreate),
				},
				Warnings: []string{},
			}, nil
		}
		return nil, err
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	status.GateBlocked, status.GateReasons = evaluatePRGate(policy, status)
	s.classifyPRLinkageStatus(cr, status)
	if status.Merged {
		if reconcileErr := s.reconcileRemoteMergedPR(cr, status); reconcileErr != nil {
			status.Warnings = append(status.Warnings, fmt.Sprintf("remote merge reconciliation failed: %v", reconcileErr))
		}
	}
	return status, nil
}

func prOpenCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr pr open <id> --approve-open"
	}
	return fmt.Sprintf("sophia cr pr open %d --approve-open", crID)
}

func prTaskListCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr task list <id>"
	}
	return fmt.Sprintf("sophia cr task list %d", crID)
}

func prTaskDoneFromContractCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr task done <id> <task-id> --from-contract"
	}
	return fmt.Sprintf("sophia cr task done %d <task-id> --from-contract", crID)
}

func prMergeApproveOpenCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr merge <id> --approve-pr-open"
	}
	return fmt.Sprintf("sophia cr merge %d --approve-pr-open", crID)
}

func prReadyBlockedSuggestedCommands(crID int) []string {
	return cleanAndDedupeStrings([]string{
		prTaskListCommand(crID),
		prTaskDoneFromContractCommand(crID),
		prMergeApproveOpenCommand(crID),
	})
}

func hasImplementationCheckpointProgress(cr *model.CR) bool {
	if cr == nil {
		return false
	}
	for _, task := range cr.Subtasks {
		if strings.TrimSpace(task.CheckpointCommit) != "" {
			return true
		}
	}
	return false
}

func prReadyCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr pr ready <id>"
	}
	return fmt.Sprintf("sophia cr pr ready %d", crID)
}

func prUnreadyCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr pr unready <id>"
	}
	return fmt.Sprintf("sophia cr pr unready %d", crID)
}

func prCloseCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr pr close <id>"
	}
	return fmt.Sprintf("sophia cr pr close %d", crID)
}

func prReopenCommand(crID int) string {
	if crID <= 0 {
		return "sophia cr pr reopen <id>"
	}
	return fmt.Sprintf("sophia cr pr reopen %d", crID)
}

func prReconcileCommand(crID int, mode string) string {
	if crID <= 0 {
		return fmt.Sprintf("sophia cr pr reconcile <id> --mode %s", strings.TrimSpace(mode))
	}
	return fmt.Sprintf("sophia cr pr reconcile %d --mode %s", crID, strings.TrimSpace(mode))
}

func applyPRSummaryToCRLink(cr *model.CR, pr *ghPRSummary, repo string, now string) {
	if cr == nil || pr == nil {
		return
	}
	cr.PR.Provider = prProviderGitHub
	cr.PR.Repo = strings.TrimSpace(nonEmptyTrimmed(repo, cr.PR.Repo))
	cr.PR.Number = pr.Number
	cr.PR.URL = strings.TrimSpace(pr.URL)
	cr.PR.State = strings.TrimSpace(pr.State)
	cr.PR.Draft = pr.IsDraft
	cr.PR.LastHeadSHA = strings.TrimSpace(pr.HeadRefOID)
	cr.PR.LastBaseRef = strings.TrimSpace(pr.BaseRefName)
	cr.PR.LastStatusCheckedAt = strings.TrimSpace(now)
	cr.PR.AwaitingOpenApproval = false
	cr.PR.AwaitingOpenApprovalNote = ""
	cr.UpdatedAt = strings.TrimSpace(now)
}

func (s *Service) classifyPRLinkageStatus(cr *model.CR, status *PRStatusView) {
	if cr == nil || status == nil {
		return
	}
	status.LinkageState = prLinkageHealthy
	if status.Merged {
		return
	}
	state := strings.ToUpper(strings.TrimSpace(status.State))
	if state == "CLOSED" {
		status.LinkageState = prLinkageClosed
		status.ActionRequired = prActionReopenPR
		status.ActionReason = fmt.Sprintf("linked PR #%d is closed without merge", status.Number)
		status.GateBlocked = true
		status.GateReasons = cleanAndDedupeStrings(append(status.GateReasons, fmt.Sprintf("%s; run `%s`", prGateClosedReason, prReopenCommand(cr.ID))))
		status.SuggestedCommands = cleanAndDedupeStrings(append(status.SuggestedCommands, prReopenCommand(cr.ID), prReconcileCommand(cr.ID, prReconcileModeCreate)))
		return
	}
	if status.Draft {
		status.ActionRequired = nonEmptyTrimmed(status.ActionRequired, prActionReadyPR)
		status.ActionReason = nonEmptyTrimmed(status.ActionReason, fmt.Sprintf("linked PR #%d is draft", status.Number))
		status.SuggestedCommands = cleanAndDedupeStrings(append(status.SuggestedCommands, prReadyCommand(cr.ID), prUnreadyCommand(cr.ID), prCloseCommand(cr.ID)))
	}

	expectedBase := strings.TrimSpace(nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
	expectedHead := strings.TrimSpace(cr.Branch)
	mismatch := []string{}
	if expectedBase != "" && strings.TrimSpace(status.BaseRefName) != "" && !strings.EqualFold(strings.TrimSpace(status.BaseRefName), expectedBase) {
		mismatch = append(mismatch, fmt.Sprintf("base ref mismatch (expected %s, observed %s)", expectedBase, strings.TrimSpace(status.BaseRefName)))
	}
	if expectedHead != "" && strings.TrimSpace(status.HeadRefName) != "" && !strings.EqualFold(strings.TrimSpace(status.HeadRefName), expectedHead) {
		mismatch = append(mismatch, fmt.Sprintf("head ref mismatch (expected %s, observed %s)", expectedHead, strings.TrimSpace(status.HeadRefName)))
	}
	if len(mismatch) == 0 {
		return
	}
	status.LinkageState = prLinkageMismatch
	status.ActionRequired = prActionReconcilePR
	status.ActionReason = fmt.Sprintf("linked PR does not match CR linkage: %s", strings.Join(mismatch, "; "))
	status.GateBlocked = true
	status.GateReasons = cleanAndDedupeStrings(append(status.GateReasons, fmt.Sprintf("%s; run `%s`", prGateMismatchReason, prReconcileCommand(cr.ID, prReconcileModeRelink))))
	status.SuggestedCommands = cleanAndDedupeStrings(append(status.SuggestedCommands, prReconcileCommand(cr.ID, prReconcileModeRelink), prReconcileCommand(cr.ID, prReconcileModeCreate)))
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

func normalizeCheckRollupState(status, conclusion, state string) string {
	normalizedState := strings.ToUpper(strings.TrimSpace(state))
	if normalizedState != "" {
		return normalizedState
	}
	normalizedStatus := strings.ToUpper(strings.TrimSpace(status))
	if normalizedStatus == "COMPLETED" {
		normalizedConclusion := strings.ToUpper(strings.TrimSpace(conclusion))
		if normalizedConclusion != "" {
			return normalizedConclusion
		}
	}
	if normalizedStatus != "" {
		return normalizedStatus
	}
	return strings.ToUpper(strings.TrimSpace(conclusion))
}

func checkRollupStatePassing(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return true
	default:
		return false
	}
}

func (s *Service) mergePRGateCRUnlocked(id int, opts MergeCROptions, policy *model.RepoPolicy) (*MergeCRResult, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status == model.StatusAbandoned {
		return nil, fmt.Errorf("cr %d is abandoned; run `sophia cr reopen %d` before merge", id, id)
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
	status.GateBlocked, status.GateReasons = evaluatePRGate(policy, status)
	s.classifyPRLinkageStatus(cr, status)
	blocked, reasons := status.GateBlocked, status.GateReasons
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
	if cr.Status == model.StatusAbandoned {
		return nil, fmt.Errorf("cr %d is abandoned; run `sophia cr reopen %d` before merge finalize", id, id)
	}
	if cr.Status != model.StatusInProgress {
		return nil, fmt.Errorf("cr %d is not in progress", id)
	}
	if cr.PR.Number <= 0 {
		return nil, fmt.Errorf("cr %d has no linked PR", id)
	}
	status, err := s.fetchGHPRStatus(cr)
	if err != nil {
		return nil, err
	}
	status.GateBlocked, status.GateReasons = evaluatePRGate(policy, status)
	s.classifyPRLinkageStatus(cr, status)
	blocked, reasons := status.GateBlocked, status.GateReasons
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
	if _, err := s.runGH(s.ghRepoSelectorForCR(cr), buildPRMergeArgs(cr.PR.Number, !opts.KeepBranch, status.HeadRefOID)...); err != nil {
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
	b.WriteString("\n### Blast Radius\n")
	if strings.TrimSpace(cr.Contract.BlastRadius) == "" {
		b.WriteString("- WARNING: missing blast_radius\n")
	} else {
		b.WriteString(strings.TrimSpace(cr.Contract.BlastRadius) + "\n")
	}
	if len(cr.ContractDrifts) > 0 {
		b.WriteString("\n### CR Contract Drifts\n")
		renderCRContractDriftSection(&b, cr.ContractDrifts)
	}
	b.WriteString("\n### Tasks\n")
	renderTaskContractTable(&b, cr.Subtasks)

	hasTaskDrifts := false
	for _, t := range cr.Subtasks {
		if len(t.ContractDrifts) > 0 {
			hasTaskDrifts = true
			break
		}
	}
	if hasTaskDrifts {
		b.WriteString("\n### Task Contract Drifts\n")
		renderTaskContractDriftSection(&b, cr.Subtasks)
	}

	if len(cr.Evidence) > 0 {
		b.WriteString("\n### Evidence\n")
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
	renderDiffBreakdownTable(&b, review)
	b.WriteString("\n### Rollback Plan\n")
	if strings.TrimSpace(cr.Contract.RollbackPlan) == "" {
		b.WriteString("- WARNING: missing rollback_plan\n")
	} else {
		b.WriteString(strings.TrimSpace(cr.Contract.RollbackPlan) + "\n")
	}
	b.WriteString("\n<!-- sophia:managed:end -->\n")
	return b.String()
}

func renderCRContractDriftSection(b *strings.Builder, drifts []model.CRContractDrift) {
	if b == nil {
		return
	}
	if len(drifts) == 0 {
		return
	}
	items := append([]model.CRContractDrift(nil), drifts...)
	sort.SliceStable(items, func(i, j int) bool {
		left := strings.TrimSpace(items[i].TS)
		right := strings.TrimSpace(items[j].TS)
		if left == right {
			return items[i].ID < items[j].ID
		}
		return left < right
	})
	for _, drift := range items {
		ack := "no"
		if drift.Acknowledged {
			ack = "yes"
		}
		b.WriteString(fmt.Sprintf("- **Drift ID %d**\n", drift.ID))
		b.WriteString(fmt.Sprintf("  - Time: %s\n", nonEmptyTrimmed(strings.TrimSpace(drift.TS), "-")))
		b.WriteString(fmt.Sprintf("  - Change: %s\n", nonEmptyTrimmed(strings.Join(cleanAndDedupeStrings(drift.Fields), ", "), "-")))
		b.WriteString(fmt.Sprintf("  - Acknowledged: %s\n", ack))
		b.WriteString(fmt.Sprintf("  - Reason: %s\n", nonEmptyTrimmed(strings.TrimSpace(drift.Reason), "-")))
		if strings.TrimSpace(drift.AckReason) != "" {
			b.WriteString(fmt.Sprintf("  - Ack Reason: %s\n", strings.TrimSpace(drift.AckReason)))
		}
		if len(drift.BeforeScope) > 0 || len(drift.AfterScope) > 0 {
			added, removed := scopeDelta(cleanAndDedupeStrings(drift.BeforeScope), cleanAndDedupeStrings(drift.AfterScope))
			b.WriteString("  - Scope Delta:\n")
			b.WriteString(fmt.Sprintf("    - Added: %s\n", nonEmptyTrimmed(strings.Join(added, ", "), "none")))
			b.WriteString(fmt.Sprintf("    - Removed: %s\n", nonEmptyTrimmed(strings.Join(removed, ", "), "none")))
		}
	}
}

func scopeDelta(before, after []string) ([]string, []string) {
	beforeSet := stringSet(before)
	afterSet := stringSet(after)
	added := []string{}
	removed := []string{}
	for path := range afterSet {
		if _, exists := beforeSet[path]; !exists {
			added = append(added, path)
		}
	}
	for path := range beforeSet {
		if _, exists := afterSet[path]; !exists {
			removed = append(removed, path)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func renderTaskContractTable(b *strings.Builder, tasks []model.Subtask) {
	if b == nil {
		return
	}
	if len(tasks) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	items := append([]model.Subtask(nil), tasks...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	b.WriteString("| Status | Task | Scope | Checkpoint | Contract Drift |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, task := range items {
		drift := "no"
		if taskHasContractDrift(task) {
			drift = fmt.Sprintf("yes (%d)", len(task.ContractDrifts))
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			markdownTableCell(taskStatusLabel(task.Status)),
			markdownTableCell(strings.TrimSpace(task.Title)),
			markdownTableCell(markdownListCell(task.Contract.Scope)),
			markdownTableCell(shortCheckpointRef(task.CheckpointCommit)),
			markdownTableCell(drift),
		))
	}
}

func renderTaskContractDriftSection(b *strings.Builder, tasks []model.Subtask) {
	if b == nil {
		return
	}
	if len(tasks) == 0 {
		return
	}
	items := append([]model.Subtask(nil), tasks...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	emitted := 0
	for _, task := range items {
		if len(task.ContractDrifts) == 0 {
			continue
		}
		drifts := append([]model.TaskContractDrift(nil), task.ContractDrifts...)
		sort.SliceStable(drifts, func(i, j int) bool {
			left := strings.TrimSpace(drifts[i].TS)
			right := strings.TrimSpace(drifts[j].TS)
			if left == right {
				return drifts[i].ID < drifts[j].ID
			}
			return left < right
		})
		for _, drift := range drifts {
			ack := "no"
			if drift.Acknowledged {
				ack = "yes"
			}
			b.WriteString(fmt.Sprintf("- task: %s | ts: %s | fields: %s | acknowledged: %s | reason: %s | ack_reason: %s\n",
				nonEmptyTrimmed(strings.TrimSpace(task.Title), "-"),
				nonEmptyTrimmed(strings.TrimSpace(drift.TS), "-"),
				nonEmptyTrimmed(strings.Join(cleanAndDedupeStrings(drift.Fields), ", "), "-"),
				ack,
				nonEmptyTrimmed(strings.TrimSpace(drift.Reason), "-"),
				nonEmptyTrimmed(strings.TrimSpace(drift.AckReason), "-"),
			))
		}
		emitted++
	}
}

func renderDiffBreakdownTable(b *strings.Builder, review *Review) {
	if b == nil || review == nil {
		return
	}
	pathSet := map[string]struct{}{}
	for _, path := range cleanAndDedupeStrings(review.Files) {
		pathSet[path] = struct{}{}
	}
	numStatByPath := map[string]struct {
		insertions string
		deletions  string
	}{}
	for _, stat := range review.DiffNumStats {
		path := strings.TrimSpace(stat.Path)
		if path == "" {
			continue
		}
		pathSet[path] = struct{}{}
		insertions := "-"
		deletions := "-"
		if stat.Binary {
			insertions = "bin"
			deletions = "bin"
		} else {
			if stat.Insertions != nil {
				insertions = strconv.Itoa(*stat.Insertions)
			}
			if stat.Deletions != nil {
				deletions = strconv.Itoa(*stat.Deletions)
			}
		}
		numStatByPath[path] = struct {
			insertions string
			deletions  string
		}{insertions: insertions, deletions: deletions}
	}
	if len(pathSet) == 0 {
		return
	}
	newPaths := stringSet(cleanAndDedupeStrings(review.NewFiles))
	deletedPaths := stringSet(cleanAndDedupeStrings(review.DeletedFiles))
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	totalAdd := 0
	totalDel := 0
	for _, stat := range review.DiffNumStats {
		if !stat.Binary {
			if stat.Insertions != nil {
				totalAdd += *stat.Insertions
			}
			if stat.Deletions != nil {
				totalDel += *stat.Deletions
			}
		}
	}
	b.WriteString(fmt.Sprintf("\n<details><summary>File Breakdown: %d files (+%d/-%d)</summary>\n\n", len(paths), totalAdd, totalDel))
	b.WriteString("| Path | Change | + | - |\n")
	b.WriteString("| --- | --- | ---: | ---: |\n")
	for _, path := range paths {
		change := "modified"
		if _, ok := newPaths[path]; ok {
			change = "added"
		} else if _, ok := deletedPaths[path]; ok {
			change = "deleted"
		}
		insertions := "-"
		deletions := "-"
		if row, ok := numStatByPath[path]; ok {
			insertions = row.insertions
			deletions = row.deletions
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			markdownTableCell(path),
			markdownTableCell(change),
			markdownTableCell(insertions),
			markdownTableCell(deletions),
		))
	}
	b.WriteString("\n</details>\n")
}

func taskStatusLabel(status string) string {
	switch strings.TrimSpace(status) {
	case model.TaskStatusDone:
		return "done"
	case model.TaskStatusDelegated:
		return "delegated"
	default:
		return "open"
	}
}

func taskHasContractDrift(task model.Subtask) bool {
	return len(task.ContractDrifts) > 0
}

func shortCheckpointRef(commit string) string {
	trimmed := strings.TrimSpace(commit)
	if trimmed == "" {
		return "-"
	}
	if len(trimmed) > 12 {
		return trimmed[:12]
	}
	return trimmed
}

func markdownListCell(items []string) string {
	values := cleanAndDedupeStrings(items)
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, "<br>")
}

func markdownTableCell(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\n", "<br>")
	trimmed = strings.ReplaceAll(trimmed, "|", "\\|")
	return trimmed
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func cleanAndDedupeStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
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
	ctx, err := s.buildPRContextView(cr)
	if err != nil {
		return nil, err
	}
	repoURL, err := s.currentRemoteRepo("origin")
	if err != nil {
		return nil, err
	}
	repoSelector := normalizeGHRepoSelector(repoURL)
	repo := strings.TrimSpace(repoURL)
	if repoSelector != "" {
		repo = repoSelector
	}
	var pr *ghPRSummary
	if cr.PR.Number > 0 {
		pr, err = s.fetchPRByNumber(repoSelector, cr.PR.Number)
		if err != nil {
			if !isPRNotFoundError(err) {
				return nil, err
			}
			pr = nil
		}
	}
	if pr == nil {
		pr, err = s.findPRByHead(repoSelector, cr.Branch, nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch))
		if err != nil {
			return nil, err
		}
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
		if err := s.pushBranchIfNeeded(cr); err != nil {
			return nil, err
		}
		url, createErr := s.createDraftPR(repoSelector, cr, ctx.Markdown)
		if createErr != nil {
			return nil, createErr
		}
		pr = &ghPRSummary{URL: strings.TrimSpace(url)}
		if refreshed, refreshErr := s.findPRByHead(repoSelector, cr.Branch, nonEmptyTrimmed(cr.BaseRef, cr.BaseBranch)); refreshErr == nil && refreshed != nil {
			pr = refreshed
		}
	}
	if pr != nil && pr.Number <= 0 {
		if parsed := parsePRNumberFromURL(pr.URL); parsed > 0 {
			if byNumber, byNumberErr := s.fetchPRByNumber(repoSelector, parsed); byNumberErr == nil && byNumber != nil {
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
		if byNumber, byNumberErr := s.fetchPRByNumber(repoSelector, pr.Number); byNumberErr == nil && byNumber != nil {
			pr = byNumber
		}
	}

	finalBody, bodyErr := s.patchManagedBody(repoSelector, pr, ctx.Markdown)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if pr.Number > 0 {
		if err := s.editPR(repoSelector, pr.Number, cr.Title, finalBody); err != nil {
			return nil, err
		}
	}
	remoteHead := strings.TrimSpace(pr.HeadRefOID)
	if pr.Number > 0 {
		updatedHead, commentErr := s.syncCheckpointComments(repoSelector, pr.Number, cr, !hadLinkedPR, remoteHead)
		if commentErr != nil {
			return nil, commentErr
		}
		if strings.TrimSpace(updatedHead) != "" {
			remoteHead = strings.TrimSpace(updatedHead)
		}
		if refreshed, refreshErr := s.fetchPRByNumber(repoSelector, pr.Number); refreshErr == nil && refreshed != nil {
			pr = refreshed
		} else if remoteHead != "" {
			pr.HeadRefOID = remoteHead
		}
	}
	now := s.timestamp()
	cr.PR.Provider = prProviderGitHub
	cr.PR.Repo = repo
	cr.PR.Number = pr.Number
	cr.PR.URL = strings.TrimSpace(pr.URL)
	cr.PR.State = strings.TrimSpace(pr.State)
	cr.PR.Draft = pr.IsDraft
	cr.PR.LastHeadSHA = strings.TrimSpace(nonEmptyTrimmed(pr.HeadRefOID, remoteHead))
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

func (s *Service) patchManagedBody(repoSelector string, pr *ghPRSummary, managed string) (string, error) {
	if pr == nil || pr.Number <= 0 {
		return managed, nil
	}
	body, err := s.readPRBody(repoSelector, pr.Number)
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

type checkpointSyncPending struct {
	Key          string
	TaskIndex    int
	TaskID       int
	Commit       string
	MissingIndex int
}

func checkpointSyncKey(taskID int, commit string) string {
	return fmt.Sprintf("task:%d:%s", taskID, strings.TrimSpace(commit))
}

func checkpointSyncCommentMarker(key string) string {
	return fmt.Sprintf("<!-- sophia:checkpoint-sync:%s -->", strings.TrimSpace(key))
}

func extractCheckpointSyncCommentKey(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	const prefix = "<!-- sophia:checkpoint-sync:"
	const suffix = "-->"
	start := strings.LastIndex(trimmed, prefix)
	if start < 0 {
		return ""
	}
	rest := trimmed[start+len(prefix):]
	end := strings.Index(rest, suffix)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func checkpointCommentIntent(task model.Subtask) string {
	if value := strings.TrimSpace(task.ContractBaseline.Intent); value != "" {
		return value
	}
	return strings.TrimSpace(task.Contract.Intent)
}

func checkpointCommentAcceptance(task model.Subtask) []string {
	if len(task.ContractBaseline.AcceptanceCriteria) > 0 {
		return cleanAndDedupeStrings(task.ContractBaseline.AcceptanceCriteria)
	}
	return cleanAndDedupeStrings(task.Contract.AcceptanceCriteria)
}

func checkpointCommentContractScope(task model.Subtask) []string {
	if len(task.ContractBaseline.Scope) > 0 {
		return cleanAndDedupeStrings(task.ContractBaseline.Scope)
	}
	return cleanAndDedupeStrings(task.Contract.Scope)
}

func checkpointCommentCheckpointScope(task model.Subtask) []string {
	scope := cleanAndDedupeStrings(task.CheckpointScope)
	if len(scope) > 0 {
		return scope
	}
	fromChunks := make([]string, 0, len(task.CheckpointChunks))
	for _, chunk := range task.CheckpointChunks {
		path := strings.TrimSpace(chunk.Path)
		if path != "" {
			fromChunks = append(fromChunks, path)
		}
	}
	return cleanAndDedupeStrings(fromChunks)
}

func renderCheckpointSyncComment(task model.Subtask, commit, commitSubject, key string) string {
	taskTitle := strings.TrimSpace(task.Title)
	if taskTitle == "" {
		taskTitle = "Untitled task"
	}
	intent := nonEmptyTrimmed(checkpointCommentIntent(task), "-")
	acceptance := checkpointCommentAcceptance(task)
	contractScope := checkpointCommentContractScope(task)
	checkpointScope := checkpointCommentCheckpointScope(task)
	shortCommit := shortCheckpointRef(commit)
	subject := nonEmptyTrimmed(strings.TrimSpace(commitSubject), "(no subject)")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("### Checkpoint sync: task %d - %s\n\n", task.ID, taskTitle))
	b.WriteString(fmt.Sprintf("Intent: %s\n\n", intent))
	b.WriteString("Acceptance Criteria:\n")
	if len(acceptance) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, item := range acceptance {
			b.WriteString(fmt.Sprintf("- %s\n", nonEmptyTrimmed(item, "-")))
		}
	}
	b.WriteString("\nScope:\n")
	b.WriteString("| Type | Paths |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| Contract Scope | %s |\n", markdownTableCell(markdownListCell(contractScope))))
	b.WriteString(fmt.Sprintf("| Checkpoint Scope | %s |\n", markdownTableCell(markdownListCell(checkpointScope))))
	b.WriteString("\nCommits in this sync:\n")
	b.WriteString(fmt.Sprintf("- `%s` %s\n\n", shortCommit, subject))
	b.WriteString(checkpointSyncCommentMarker(key))
	b.WriteString("\n")
	return b.String()
}

func normalizeRenderedCommentBody(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
}

type ghIssueComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

func indexCheckpointSyncComments(comments []ghIssueComment) map[string]ghIssueComment {
	index := map[string]ghIssueComment{}
	for _, comment := range comments {
		key := extractCheckpointSyncCommentKey(comment.Body)
		if key == "" {
			continue
		}
		index[key] = comment
	}
	return index
}

func parseRevListOutput(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func validateCheckpointStrictOrder(toSync []checkpointSyncPending, missingCommits []string) error {
	lastMissingIndex := -1
	for _, pending := range toSync {
		if pending.MissingIndex == lastMissingIndex {
			continue
		}
		if pending.MissingIndex != lastMissingIndex+1 {
			blocking := ""
			if lastMissingIndex+1 >= 0 && lastMissingIndex+1 < len(missingCommits) {
				blocking = shortCheckpointRef(missingCommits[lastMissingIndex+1])
			}
			if blocking == "" {
				blocking = "non-checkpoint commit"
			}
			return fmt.Errorf("checkpoint sync for task %d requires a clean checkpoint-only branch order; commit %s appears before checkpoint commit %s", pending.TaskID, blocking, shortCheckpointRef(pending.Commit))
		}
		lastMissingIndex = pending.MissingIndex
	}
	return nil
}

func (s *Service) revListReverse(from, to string) ([]string, error) {
	left := strings.TrimSpace(from)
	right := strings.TrimSpace(to)
	if left == "" || right == "" || left == right {
		return []string{}, nil
	}
	out, err := s.runCommand("git", "rev-list", "--reverse", fmt.Sprintf("%s..%s", left, right))
	if err != nil {
		return nil, err
	}
	return parseRevListOutput(out), nil
}

func (s *Service) remoteBranchHeadOID(branch string) (string, error) {
	trimmed := strings.TrimSpace(branch)
	if trimmed == "" {
		return "", fmt.Errorf("branch is required")
	}
	out, err := s.runCommand("git", "ls-remote", "--heads", "origin", trimmed)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return "", nil
	}
	return strings.TrimSpace(fields[0]), nil
}

func (s *Service) resolveCommitOID(ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", fmt.Errorf("commit ref is required")
	}
	out, err := s.runCommand("git", "rev-parse", "--verify", trimmed+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) commitSubject(commit string) (string, error) {
	resolved := strings.TrimSpace(commit)
	if resolved == "" {
		return "", fmt.Errorf("commit is required")
	}
	out, err := s.runCommand("git", "show", "-s", "--format=%s", resolved)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (s *Service) listIssueComments(repoSelector string, prNumber int) ([]ghIssueComment, error) {
	if prNumber <= 0 {
		return nil, fmt.Errorf("pr number is required")
	}
	host, owner, repo, ok := parseRepoSelectorParts(repoSelector)
	if !ok {
		return nil, fmt.Errorf("unable to resolve repo selector %q for gh api comments", strings.TrimSpace(repoSelector))
	}
	args := []string{"api"}
	if host != "" {
		args = append(args, "--hostname", host)
	}
	args = append(args, fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100", owner, repo, prNumber))
	out, err := s.runCommand("gh", args...)
	if err != nil {
		return nil, classifyGHCommandError(err, args)
	}
	var comments []ghIssueComment
	if unmarshalErr := json.Unmarshal([]byte(out), &comments); unmarshalErr != nil {
		return nil, fmt.Errorf("parse issue comments output: %w", unmarshalErr)
	}
	return comments, nil
}

func (s *Service) editIssueComment(repoSelector string, commentID int64, body string) error {
	if commentID <= 0 {
		return fmt.Errorf("comment id is required")
	}
	host, owner, repo, ok := parseRepoSelectorParts(repoSelector)
	if !ok {
		return fmt.Errorf("unable to resolve repo selector %q for gh api comment edit", strings.TrimSpace(repoSelector))
	}
	payload := map[string]string{
		"body": body,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	args := []string{"api"}
	if host != "" {
		args = append(args, "--hostname", host)
	}
	args = append(args,
		fmt.Sprintf("repos/%s/%s/issues/comments/%d", owner, repo, commentID),
		"-X", "PATCH",
		"--input", "-",
	)
	cmd := exec.Command("gh", args...)
	cmd.Dir = s.git.WorkDir
	cmd.Stdin = strings.NewReader(string(raw))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = runErr.Error()
		}
		commandErr := fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), runErr, msg)
		return classifyGHCommandError(commandErr, args)
	}
	return nil
}

func (s *Service) upsertCheckpointSyncComment(repoSelector string, prNumber int, task model.Subtask, commit, key string, index map[string]ghIssueComment) error {
	subject, err := s.commitSubject(commit)
	if err != nil {
		return err
	}
	body := renderCheckpointSyncComment(task, commit, subject, key)
	if existing, ok := index[key]; ok {
		if normalizeRenderedCommentBody(existing.Body) == normalizeRenderedCommentBody(body) {
			return nil
		}
		if editErr := s.editIssueComment(repoSelector, existing.ID, body); editErr != nil {
			return editErr
		}
		index[key] = ghIssueComment{ID: existing.ID, Body: body}
		return nil
	}
	if _, err := s.runGH(repoSelector, "pr", "comment", strconv.Itoa(prNumber), "--body", body); err != nil {
		return err
	}
	index[key] = ghIssueComment{ID: 0, Body: body}
	return nil
}

func (s *Service) pushBranchRefspec(refspec string, dryRun bool) error {
	trimmed := strings.TrimSpace(refspec)
	if trimmed == "" {
		return fmt.Errorf("push refspec is required")
	}
	args := []string{"push"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, "origin", trimmed)
	_, err := s.runCommand("git", args...)
	return err
}

func (s *Service) gitIsAncestor(ancestor, descendant string) (bool, error) {
	left := strings.TrimSpace(ancestor)
	right := strings.TrimSpace(descendant)
	if left == "" || right == "" {
		return false, fmt.Errorf("ancestor and descendant refs are required")
	}
	cmd := exec.Command("git", "merge-base", "--is-ancestor", left, right)
	cmd.Dir = s.git.WorkDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return false, fmt.Errorf("git merge-base --is-ancestor %s %s: %s", left, right, msg)
	}
	return true, nil
}

func (s *Service) syncCheckpointComments(repoSelector string, prNumber int, cr *model.CR, skipPosting bool, remoteHeadSHA string) (string, error) {
	if cr == nil || prNumber <= 0 {
		return strings.TrimSpace(remoteHeadSHA), nil
	}
	remoteHead := strings.TrimSpace(remoteHeadSHA)
	if remoteHead == "" {
		resolved, err := s.remoteBranchHeadOID(cr.Branch)
		if err != nil {
			return "", err
		}
		remoteHead = strings.TrimSpace(resolved)
	}
	localHead, err := s.branchHeadOID(cr.Branch)
	if err != nil {
		return "", err
	}
	localHead = strings.TrimSpace(localHead)
	if localHead == "" {
		return remoteHead, nil
	}
	if remoteHead == "" {
		// Branch not yet on remote; initial open/create path handles publication.
		return localHead, nil
	}

	missingCommits, err := s.revListReverse(remoteHead, localHead)
	if err != nil {
		return "", err
	}
	missingIndex := map[string]int{}
	for i, commit := range missingCommits {
		missingIndex[strings.TrimSpace(commit)] = i
	}

	syncedByPush := map[string]struct{}{}
	for _, key := range cr.PR.CheckpointSyncKeys {
		syncedByPush[strings.TrimSpace(key)] = struct{}{}
	}
	commentedByKey := map[string]struct{}{}
	for _, key := range cr.PR.CheckpointCommentKeys {
		commentedByKey[strings.TrimSpace(key)] = struct{}{}
	}
	commentIndex := map[string]ghIssueComment{}
	if !skipPosting {
		comments, commentsErr := s.listIssueComments(repoSelector, prNumber)
		if commentsErr != nil {
			return "", commentsErr
		}
		commentIndex = indexCheckpointSyncComments(comments)
	}

	toSync := make([]checkpointSyncPending, 0, len(cr.Subtasks))
	now := s.timestamp()
	for idx := range cr.Subtasks {
		task := &cr.Subtasks[idx]
		if task.Status != model.TaskStatusDone {
			continue
		}
		rawCommit := strings.TrimSpace(task.CheckpointCommit)
		if rawCommit == "" {
			continue
		}
		key := checkpointSyncKey(task.ID, rawCommit)
		commit, resolveErr := s.resolveCommitOID(rawCommit)
		if resolveErr != nil {
			return "", resolveErr
		}
		if _, pushed := syncedByPush[key]; pushed {
			if !skipPosting {
				if _, commented := commentedByKey[key]; !commented {
					if err := s.upsertCheckpointSyncComment(repoSelector, prNumber, *task, commit, key, commentIndex); err != nil {
						return "", err
					}
					cr.PR.CheckpointCommentKeys = append(cr.PR.CheckpointCommentKeys, key)
					commentedByKey[key] = struct{}{}
				}
			}
			continue
		}
		if pos, exists := missingIndex[commit]; exists {
			toSync = append(toSync, checkpointSyncPending{
				Key:          key,
				TaskIndex:    idx,
				TaskID:       task.ID,
				Commit:       commit,
				MissingIndex: pos,
			})
			continue
		}
		ancestor, ancestorErr := s.gitIsAncestor(commit, remoteHead)
		if ancestorErr != nil {
			return "", ancestorErr
		}
		if ancestor {
			if !skipPosting {
				if _, commented := commentedByKey[key]; !commented {
					if err := s.upsertCheckpointSyncComment(repoSelector, prNumber, *task, commit, key, commentIndex); err != nil {
						return "", err
					}
					cr.PR.CheckpointCommentKeys = append(cr.PR.CheckpointCommentKeys, key)
					commentedByKey[key] = struct{}{}
				}
			}
			cr.PR.CheckpointSyncKeys = append(cr.PR.CheckpointSyncKeys, key)
			cr.Subtasks[idx].CheckpointSyncAt = now
			syncedByPush[key] = struct{}{}
			continue
		}
		return "", fmt.Errorf("checkpoint sync requires task %d commit %s to be reachable from local or remote branch history", task.ID, shortCheckpointRef(commit))
	}

	sort.SliceStable(toSync, func(i, j int) bool {
		if toSync[i].MissingIndex != toSync[j].MissingIndex {
			return toSync[i].MissingIndex < toSync[j].MissingIndex
		}
		return toSync[i].TaskID < toSync[j].TaskID
	})

	if err := validateCheckpointStrictOrder(toSync, missingCommits); err != nil {
		return "", err
	}

	lastPushedIndex := -1
	for _, pending := range toSync {
		task := cr.Subtasks[pending.TaskIndex]
		if pending.MissingIndex == lastPushedIndex {
			if !skipPosting {
				if _, commented := commentedByKey[pending.Key]; !commented {
					if err := s.upsertCheckpointSyncComment(repoSelector, prNumber, task, pending.Commit, pending.Key, commentIndex); err != nil {
						return "", err
					}
					cr.PR.CheckpointCommentKeys = append(cr.PR.CheckpointCommentKeys, pending.Key)
					commentedByKey[pending.Key] = struct{}{}
				}
			}
			cr.PR.CheckpointSyncKeys = append(cr.PR.CheckpointSyncKeys, pending.Key)
			cr.Subtasks[pending.TaskIndex].CheckpointSyncAt = now
			syncedByPush[pending.Key] = struct{}{}
			continue
		}
		refspec := fmt.Sprintf("%s:refs/heads/%s", pending.Commit, strings.TrimSpace(cr.Branch))
		if err := s.pushBranchRefspec(refspec, true); err != nil {
			return "", err
		}
		if !skipPosting {
			if _, commented := commentedByKey[pending.Key]; !commented {
				if err := s.upsertCheckpointSyncComment(repoSelector, prNumber, task, pending.Commit, pending.Key, commentIndex); err != nil {
					return "", err
				}
				cr.PR.CheckpointCommentKeys = append(cr.PR.CheckpointCommentKeys, pending.Key)
				commentedByKey[pending.Key] = struct{}{}
			}
		}
		if err := s.pushBranchRefspec(refspec, false); err != nil {
			return "", err
		}
		remoteHead = pending.Commit
		cr.PR.CheckpointSyncKeys = append(cr.PR.CheckpointSyncKeys, pending.Key)
		cr.Subtasks[pending.TaskIndex].CheckpointSyncAt = now
		syncedByPush[pending.Key] = struct{}{}
		lastPushedIndex = pending.MissingIndex
	}

	if strings.TrimSpace(remoteHead) != localHead {
		refspec := fmt.Sprintf("%s:refs/heads/%s", localHead, strings.TrimSpace(cr.Branch))
		if err := s.pushBranchRefspec(refspec, true); err != nil {
			return "", err
		}
		if err := s.pushBranchRefspec(refspec, false); err != nil {
			return "", err
		}
		remoteHead = localHead
	}

	sort.Strings(cr.PR.CheckpointCommentKeys)
	cr.PR.CheckpointCommentKeys = dedupeStrings(cr.PR.CheckpointCommentKeys)
	sort.Strings(cr.PR.CheckpointSyncKeys)
	cr.PR.CheckpointSyncKeys = dedupeStrings(cr.PR.CheckpointSyncKeys)
	return remoteHead, nil
}

func (s *Service) findPRByHead(repoSelector, branch, baseRef string) (*ghPRSummary, error) {
	if strings.TrimSpace(branch) == "" {
		return nil, fmt.Errorf("branch is required")
	}
	out, err := s.runGH(repoSelector, "pr", "list", "--head", branch, "--state", "all", "--json", "number,url,state,isDraft,headRefOid,headRefName,baseRefName,reviewDecision,mergedAt,mergeCommit,updatedAt")
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
	candidates := append([]ghPRSummary(nil), items...)
	baseRef = strings.TrimSpace(baseRef)
	if baseRef != "" {
		filtered := make([]ghPRSummary, 0, len(candidates))
		for _, item := range candidates {
			if strings.EqualFold(strings.TrimSpace(item.BaseRefName), baseRef) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 1 {
			return &filtered[0], nil
		}
		if len(filtered) > 1 {
			candidates = filtered
		}
	}
	if headOID, headErr := s.branchHeadOID(branch); headErr == nil && strings.TrimSpace(headOID) != "" {
		filtered := make([]ghPRSummary, 0, len(candidates))
		for _, item := range candidates {
			if strings.EqualFold(strings.TrimSpace(item.HeadRefOID), strings.TrimSpace(headOID)) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 1 {
			return &filtered[0], nil
		}
		if len(filtered) > 1 {
			candidates = filtered
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := strings.TrimSpace(candidates[i].UpdatedAt)
		right := strings.TrimSpace(candidates[j].UpdatedAt)
		if left == right {
			return candidates[i].Number > candidates[j].Number
		}
		return left > right
	})
	return &candidates[0], nil
}

func (s *Service) fetchPRByNumber(repoSelector string, number int) (*ghPRSummary, error) {
	if number <= 0 {
		return nil, fmt.Errorf("pr number is required")
	}
	out, err := s.runGH(repoSelector, "pr", "view", strconv.Itoa(number), "--json", "number,url,state,isDraft,headRefOid,headRefName,baseRefName,reviewDecision,mergedAt,mergeCommit")
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

func (s *Service) createDraftPR(repoSelector string, cr *model.CR, body string) (string, error) {
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
	out, err := s.runGH(repoSelector, "pr", "create", "--draft", "--title", strings.TrimSpace(cr.Title), "--body-file", bodyFile, "--base", base, "--head", strings.TrimSpace(cr.Branch))
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

func (s *Service) editPR(repoSelector string, number int, title, body string) error {
	if number <= 0 {
		return fmt.Errorf("pr number is required")
	}
	bodyFile, err := writeTempBody(body)
	if err != nil {
		return err
	}
	defer os.Remove(bodyFile)
	_, err = s.runGH(repoSelector, "pr", "edit", strconv.Itoa(number), "--title", strings.TrimSpace(title), "--body-file", bodyFile)
	if err == nil {
		return nil
	}
	if isGHProjectCardsSunsetError(err) {
		if fallbackErr := s.editPRViaAPI(repoSelector, number, title, body); fallbackErr == nil {
			return nil
		} else {
			return fallbackErr
		}
	}
	return err
}

func (s *Service) editPRViaAPI(repoSelector string, number int, title, body string) error {
	host, owner, repo, ok := parseRepoSelectorParts(repoSelector)
	if !ok {
		return fmt.Errorf("unable to resolve repo selector %q for gh api fallback", strings.TrimSpace(repoSelector))
	}
	payload := map[string]string{
		"title": strings.TrimSpace(title),
		"body":  body,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	args := []string{"api"}
	if host != "" {
		args = append(args, "--hostname", host)
	}
	args = append(args,
		fmt.Sprintf("repos/%s/%s/pulls/%d", owner, repo, number),
		"-X", "PATCH",
		"--input", "-",
	)
	cmd := exec.Command("gh", args...)
	cmd.Dir = s.git.WorkDir
	cmd.Stdin = strings.NewReader(string(raw))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = runErr.Error()
		}
		commandErr := fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), runErr, msg)
		return classifyGHCommandError(commandErr, args)
	}
	return nil
}

func parseRepoSelectorParts(repoSelector string) (host string, owner string, repo string, ok bool) {
	selector := strings.TrimSpace(normalizeGHRepoSelector(repoSelector))
	if selector == "" {
		return "", "", "", false
	}
	parts := strings.Split(selector, "/")
	switch len(parts) {
	case 2:
		owner = strings.TrimSpace(parts[0])
		repo = strings.TrimSpace(parts[1])
		if owner == "" || repo == "" {
			return "", "", "", false
		}
		return "", owner, repo, true
	case 3:
		host = strings.TrimSpace(parts[0])
		owner = strings.TrimSpace(parts[1])
		repo = strings.TrimSpace(parts[2])
		if host == "" || owner == "" || repo == "" {
			return "", "", "", false
		}
		return host, owner, repo, true
	default:
		return "", "", "", false
	}
}

func isGHProjectCardsSunsetError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "projects (classic) is being deprecated") &&
		strings.Contains(lower, "repository.pullrequest.projectcards")
}

func (s *Service) readPRBody(repoSelector string, number int) (string, error) {
	out, err := s.runGH(repoSelector, "pr", "view", strconv.Itoa(number), "--json", "body")
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
	out, err := s.runGH(s.ghRepoSelectorForCR(cr), "pr", "view", strconv.Itoa(cr.PR.Number), "--json", "number,url,state,isDraft,reviewDecision,mergedAt,mergeCommit,author,latestReviews,statusCheckRollup,headRefOid,headRefName,baseRefName")
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
			state := normalizeCheckRollupState(check.Status, check.Conclusion, check.State)
			if !checkRollupStatePassing(state) {
				checksPassing = false
				break
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
		HeadRefOID:         strings.TrimSpace(pr.HeadRefOID),
		HeadRefName:        strings.TrimSpace(pr.HeadRefName),
		BaseRefName:        strings.TrimSpace(pr.BaseRefName),
		Approvals:          approvals,
		NonAuthorApprovals: nonAuthor,
		LinkageState:       prLinkageHealthy,
		GateReasons:        []string{},
		SuggestedCommands:  []string{},
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
	// Always push before PR create/sync so existing PRs receive local-ahead commits.
	// This is idempotent when branch state is unchanged.
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
			return s.newBranchInOtherWorktreeError(cr.ID, branch, owner.Path, "pr_stage_archive", fmt.Sprintf("sophia cr pr sync %d", cr.ID))
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
	fullDiff, err := buildArchiveFullDiffFromCachedDiff(mergeGit, gitSummary.FilesChanged, archivePolicyIncludeFullDiffs(policy.Archive))
	if err != nil {
		return err
	}
	archiveCR := *cr
	archiveCR.Status = model.StatusMerged
	archiveCR.MergedAt = s.timestamp()
	archiveCR.MergedBy = mergeGit.Actor()
	archive := buildCRArchiveDocument(&archiveCR, 1, "", archiveCR.MergedAt, policy.Archive, gitSummary, fullDiff)
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

func (s *Service) runGH(repoSelector string, args ...string) (string, error) {
	selector := strings.TrimSpace(repoSelector)
	cmdArgs := make([]string, 0, len(args)+2)
	if selector != "" {
		cmdArgs = append(cmdArgs, "-R", selector)
	}
	cmdArgs = append(cmdArgs, args...)
	return s.runCommand("gh", cmdArgs...)
}

func (s *Service) ghRepoSelectorForCR(cr *model.CR) string {
	if cr != nil {
		if selector := normalizeGHRepoSelector(cr.PR.Repo); selector != "" {
			return selector
		}
	}
	remoteURL, err := s.currentRemoteRepo("origin")
	if err != nil {
		return ""
	}
	return normalizeGHRepoSelector(remoteURL)
}

func normalizeGHRepoSelector(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if parsed := parseGitHubRepoSelector(trimmed); parsed != "" {
		return parsed
	}
	if isValidGHRepoSelector(trimmed) {
		return trimmed
	}
	return ""
}

func parseGitHubRepoSelector(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	var (
		host string
		path string
	)
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return ""
		}
		host = strings.TrimSpace(u.Host)
		path = strings.TrimSpace(u.Path)
	} else if at := strings.Index(trimmed, "@"); at >= 0 && strings.Contains(trimmed[at+1:], ":") {
		right := trimmed[at+1:]
		parts := strings.SplitN(right, ":", 2)
		if len(parts) != 2 {
			return ""
		}
		host = strings.TrimSpace(parts[0])
		path = strings.TrimSpace(parts[1])
	} else {
		return ""
	}
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	owner := strings.TrimSpace(parts[0])
	repo := strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	if owner == "" || repo == "" {
		return ""
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if host == "github.com" {
		return owner + "/" + repo
	}
	return host + "/" + owner + "/" + repo
}

func isValidGHRepoSelector(selector string) bool {
	parts := strings.Split(strings.TrimSpace(selector), "/")
	if len(parts) != 2 && len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return false
		}
		if strings.ContainsAny(part, " :\\") {
			return false
		}
	}
	return true
}

func isPRNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "no pull requests found") ||
		strings.Contains(lower, "pull request not found") ||
		strings.Contains(lower, "could not resolve to a pullrequest")
}

func (s *Service) branchHeadOID(branch string) (string, error) {
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("branch is required")
	}
	head, err := s.runCommand("git", "rev-parse", strings.TrimSpace(branch))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(head), nil
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

func buildPRMergeArgs(number int, deleteBranch bool, expectedHeadOID string) []string {
	args := []string{"pr", "merge", strconv.Itoa(number), "--merge"}
	if strings.TrimSpace(expectedHeadOID) != "" {
		args = append(args, "--match-head-commit", strings.TrimSpace(expectedHeadOID))
	}
	if deleteBranch {
		args = append(args, "--delete-branch")
	}
	return args
}
