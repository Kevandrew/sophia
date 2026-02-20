package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// HQSyncStatusView is a read-only, agent-friendly snapshot of local-vs-remote intent sync state.
// It is intentionally derived from intent-only fingerprints (no events/checkpoints/evidence).
type HQSyncStatusView struct {
	Configured          bool
	BaseURL             string
	RepoID              string
	RemoteAlias         string
	HasToken            bool
	Linked              bool
	LocalFingerprint    string
	UpstreamFingerprint string
	RemoteExists        bool
	RemoteChecked       bool
	RemoteFingerprint   string
	State               string
	SuggestedActions    []string
}

const (
	hqSyncStateNotConfigured = "not_configured"
	hqSyncStateRemoteMissing = "remote_missing"
	hqSyncStateUnlinked      = "unlinked"
	hqSyncStateUpToDate      = "up_to_date"
	hqSyncStateLocalAhead    = "local_ahead"
	hqSyncStateRemoteAhead   = "remote_ahead"
	hqSyncStateDiverged      = "diverged"
)

func (s *Service) HQSyncStatusCR(id int) (*HQSyncStatusView, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}

	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}

	view := &HQSyncStatusView{
		BaseURL:     strings.TrimSpace(resolved.BaseURL),
		RepoID:      strings.TrimSpace(resolved.RepoID),
		RemoteAlias: strings.TrimSpace(resolved.RemoteAlias),
		HasToken:    strings.TrimSpace(resolved.Token) != "",
	}

	localFingerprint, err := fingerprintHQIntentCR(cr)
	if err != nil {
		return nil, err
	}
	view.LocalFingerprint = strings.TrimSpace(localFingerprint)

	upstream := strings.TrimSpace(cr.HQ.UpstreamFingerprint)
	view.UpstreamFingerprint = upstream
	view.Linked = upstream != ""

	// Consider HQ "configured" when repo identity is present. Base URL/remote alias are always resolved
	// via defaults, so repo_id is the decision boundary for whether remote sync is meaningful.
	if strings.TrimSpace(resolved.RepoID) == "" {
		view.Configured = false
		view.RemoteChecked = false
		view.RemoteExists = false
		view.State = hqSyncStateNotConfigured
		view.SuggestedActions = []string{
			"sophia hq config set --repo-id <org/repo>",
			"sophia hq login --token-stdin",
		}
		return view, nil
	}
	view.Configured = true

	uid := strings.TrimSpace(cr.UID)
	if uid == "" {
		view.RemoteChecked = false
		view.RemoteExists = false
		view.State = hqSyncStateUnlinked
		view.SuggestedActions = []string{
			fmt.Sprintf("sophia repair"),
		}
		return view, nil
	}

	// Status must be safe for agent loops: keep the remote check bounded.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := newHQClient(resolved.BaseURL, resolved.Token)
	view.RemoteChecked = true
	remoteCR, fetchErr := client.GetCR(ctx, resolved.RepoID, uid)
	if fetchErr != nil {
		view.RemoteExists = false
		var remoteErr *HQRemoteError
		if errors.As(fetchErr, &remoteErr) && remoteErr.StatusCode == 404 {
			view.State = hqSyncStateRemoteMissing
			view.SuggestedActions = []string{fmt.Sprintf("sophia cr push %d", cr.ID)}
			return view, nil
		}

		actions := make([]string, 0, 3)
		if !view.HasToken {
			actions = append(actions, "sophia hq login --token-stdin")
		}
		actions = append(actions,
			"sophia hq config show --json",
			fmt.Sprintf("sophia hq cr show %s --json", uid),
		)
		view.State = hqSyncStateUnlinked
		view.SuggestedActions = actions
		return view, nil
	}

	view.RemoteExists = true
	remoteFingerprint := strings.TrimSpace(remoteCR.CRFingerprint)
	if remoteFingerprint == "" {
		docRaw := remoteCR.Doc
		if len(docRaw) == 0 {
			docRaw = remoteCR.CR
		}
		if len(docRaw) == 0 {
			// Treat as remote missing/malformed for status purposes.
			view.RemoteFingerprint = ""
			view.State = hqSyncStateRemoteMissing
			view.SuggestedActions = []string{fmt.Sprintf("sophia hq cr show %s --json", uid)}
			return view, nil
		}
		var doc CRDoc
		if err := json.Unmarshal(docRaw, &doc); err != nil {
			return nil, fmt.Errorf("decode remote CR doc: %w", err)
		}
		decodedCR := crFromDoc(&doc)
		if decodedCR == nil {
			return nil, fmt.Errorf("%w: invalid remote CR doc payload", ErrHQRemoteMalformedResponse)
		}
		remoteFingerprint, err = fingerprintHQIntentCR(decodedCR)
		if err != nil {
			return nil, err
		}
	}
	view.RemoteFingerprint = strings.TrimSpace(remoteFingerprint)

	// Compute canonical sync state with upstream guardrails.
	if !view.Linked {
		view.State = hqSyncStateUnlinked
		view.SuggestedActions = []string{
			fmt.Sprintf("sophia cr pull %d", cr.ID),
			fmt.Sprintf("sophia cr push %d --force", cr.ID),
		}
		return view, nil
	}

	localFP := view.LocalFingerprint
	upstreamFP := view.UpstreamFingerprint
	remoteFP := view.RemoteFingerprint

	switch {
	case localFP == upstreamFP && remoteFP == upstreamFP:
		view.State = hqSyncStateUpToDate
		view.SuggestedActions = []string{}
	case localFP != upstreamFP && remoteFP == upstreamFP:
		view.State = hqSyncStateLocalAhead
		view.SuggestedActions = []string{fmt.Sprintf("sophia cr push %d", cr.ID)}
	case localFP == upstreamFP && remoteFP != upstreamFP:
		view.State = hqSyncStateRemoteAhead
		view.SuggestedActions = []string{fmt.Sprintf("sophia cr pull %d", cr.ID)}
	default:
		view.State = hqSyncStateDiverged
		view.SuggestedActions = []string{
			fmt.Sprintf("sophia cr pull %d --force", cr.ID),
			fmt.Sprintf("sophia cr push %d --force", cr.ID),
		}
	}

	return view, nil
}
