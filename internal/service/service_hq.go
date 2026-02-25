package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sophia/internal/model"
	"sophia/internal/store"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultHQRemoteAlias = "hq"
	defaultHQBaseURL     = "https://sophiahq.com"
	hqConfigVersionV1    = "v1"
)

type hqGlobalConfig struct {
	Version   string `yaml:"version,omitempty"`
	HQRemote  string `yaml:"hq_remote,omitempty"`
	HQRepoID  string `yaml:"hq_repo_id,omitempty"`
	HQBaseURL string `yaml:"hq_base_url,omitempty"`
}

type hqCredentialFile struct {
	Version string            `yaml:"version,omitempty"`
	Tokens  map[string]string `yaml:"tokens,omitempty"`
}

type hqRuntimeConfig struct {
	RemoteAlias  string
	RepoID       string
	BaseURL      string
	Token        string
	MetadataMode string
}

type HQConfigValues struct {
	RemoteAlias string
	RepoID      string
	BaseURL     string
}

type HQConfigView struct {
	RemoteAlias  string
	RepoID       string
	BaseURL      string
	MetadataMode string
	TokenPresent bool
	RepoConfig   HQConfigValues
	GlobalConfig HQConfigValues
}

type HQConfigSetOptions struct {
	RemoteAlias *string
	RepoID      *string
	BaseURL     *string
	Global      bool
}

type HQCRDetail struct {
	UID         string
	Fingerprint string
	CR          *model.CR
}

type HQSyncResult struct {
	LocalCRID int
	CRUID     string
	Created   bool
	Replaced  bool
}

type HQRemoteError struct {
	StatusCode int
	Method     string
	URL        string
	Message    string
}

func (e *HQRemoteError) Error() string {
	if e == nil {
		return "hq request failed"
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("hq request failed (%s %s): %d %s", e.Method, e.URL, e.StatusCode, message)
}

func (e *HQRemoteError) Details() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return map[string]any{
		"status_code": e.StatusCode,
		"method":      e.Method,
		"url":         e.URL,
		"message":     e.Message,
	}
}

func (s *Service) GetHQConfig() (*HQConfigView, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	globalCfg, err := s.loadHQGlobalConfig()
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	return &HQConfigView{
		RemoteAlias:  resolved.RemoteAlias,
		RepoID:       resolved.RepoID,
		BaseURL:      resolved.BaseURL,
		MetadataMode: cfg.MetadataMode,
		TokenPresent: strings.TrimSpace(resolved.Token) != "",
		RepoConfig: HQConfigValues{
			RemoteAlias: strings.TrimSpace(cfg.HQRemote),
			RepoID:      strings.TrimSpace(cfg.HQRepoID),
			BaseURL:     strings.TrimSpace(cfg.HQBaseURL),
		},
		GlobalConfig: HQConfigValues{
			RemoteAlias: strings.TrimSpace(globalCfg.HQRemote),
			RepoID:      strings.TrimSpace(globalCfg.HQRepoID),
			BaseURL:     strings.TrimSpace(globalCfg.HQBaseURL),
		},
	}, nil
}

func (s *Service) SetHQConfig(opts HQConfigSetOptions) ([]string, error) {
	if opts.RemoteAlias == nil && opts.RepoID == nil && opts.BaseURL == nil {
		return nil, ErrNoCRChanges
	}
	if opts.Global {
		globalCfg, err := s.loadHQGlobalConfig()
		if err != nil {
			return nil, err
		}
		changed := []string{}
		if opts.RemoteAlias != nil {
			normalized, normalizeErr := normalizeHQRemoteAlias(*opts.RemoteAlias)
			if normalizeErr != nil {
				return nil, normalizeErr
			}
			if strings.TrimSpace(globalCfg.HQRemote) != normalized {
				globalCfg.HQRemote = normalized
				changed = append(changed, "hq_remote")
			}
		}
		if opts.RepoID != nil {
			normalized, normalizeErr := normalizeHQRepoID(*opts.RepoID)
			if normalizeErr != nil {
				return nil, normalizeErr
			}
			if strings.TrimSpace(globalCfg.HQRepoID) != normalized {
				globalCfg.HQRepoID = normalized
				changed = append(changed, "hq_repo_id")
			}
		}
		if opts.BaseURL != nil {
			normalized, normalizeErr := normalizeHQBaseURL(*opts.BaseURL)
			if normalizeErr != nil {
				return nil, normalizeErr
			}
			if strings.TrimSpace(globalCfg.HQBaseURL) != normalized {
				globalCfg.HQBaseURL = normalized
				changed = append(changed, "hq_base_url")
			}
		}
		if len(changed) == 0 {
			return nil, ErrNoCRChanges
		}
		if err := s.saveHQGlobalConfig(globalCfg); err != nil {
			return nil, err
		}
		return changed, nil
	}

	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	changed := []string{}
	if opts.RemoteAlias != nil {
		normalized, normalizeErr := normalizeHQRemoteAlias(*opts.RemoteAlias)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if strings.TrimSpace(cfg.HQRemote) != normalized {
			cfg.HQRemote = normalized
			changed = append(changed, "hq_remote")
		}
	}
	if opts.RepoID != nil {
		normalized, normalizeErr := normalizeHQRepoID(*opts.RepoID)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if strings.TrimSpace(cfg.HQRepoID) != normalized {
			cfg.HQRepoID = normalized
			changed = append(changed, "hq_repo_id")
		}
	}
	if opts.BaseURL != nil {
		normalized, normalizeErr := normalizeHQBaseURL(*opts.BaseURL)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if strings.TrimSpace(cfg.HQBaseURL) != normalized {
			cfg.HQBaseURL = normalized
			changed = append(changed, "hq_base_url")
		}
	}
	if len(changed) == 0 {
		return nil, ErrNoCRChanges
	}
	if err := s.store.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return changed, nil
}

func (s *Service) HQLogin(remoteAlias, token string) (string, error) {
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return "", fmt.Errorf("hq token cannot be empty")
	}
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return "", err
	}
	alias := strings.TrimSpace(remoteAlias)
	if alias == "" {
		alias = resolved.RemoteAlias
	}
	alias, err = normalizeHQRemoteAlias(alias)
	if err != nil {
		return "", err
	}
	creds, err := s.loadHQCredentials()
	if err != nil {
		return "", err
	}
	if creds.Tokens == nil {
		creds.Tokens = map[string]string{}
	}
	creds.Tokens[alias] = normalizedToken
	if err := s.saveHQCredentials(creds); err != nil {
		return "", err
	}
	return alias, nil
}

func (s *Service) HQLogout(remoteAlias string) (string, error) {
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return "", err
	}
	alias := strings.TrimSpace(remoteAlias)
	if alias == "" {
		alias = resolved.RemoteAlias
	}
	alias, err = normalizeHQRemoteAlias(alias)
	if err != nil {
		return "", err
	}
	creds, err := s.loadHQCredentials()
	if err != nil {
		return "", err
	}
	if len(creds.Tokens) == 0 {
		return alias, nil
	}
	delete(creds.Tokens, alias)
	if err := s.saveHQCredentials(creds); err != nil {
		return "", err
	}
	return alias, nil
}

func (s *Service) HQListCRs() ([]model.HQCRSummary, error) {
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.RepoID) == "" {
		return nil, ErrHQRepoIDRequired
	}
	client := newHQClient(resolved.BaseURL, resolved.Token)
	return client.ListCRs(context.Background(), resolved.RepoID)
}

func (s *Service) HQGetCR(uid string) (*HQCRDetail, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, fmt.Errorf("cr uid cannot be empty")
	}
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.RepoID) == "" {
		return nil, ErrHQRepoIDRequired
	}
	client := newHQClient(resolved.BaseURL, resolved.Token)
	remoteCR, err := client.GetCR(context.Background(), resolved.RepoID, uid)
	if err != nil {
		return nil, err
	}
	docRaw := remoteCR.Doc
	if len(docRaw) == 0 {
		docRaw = remoteCR.CR
	}
	if len(docRaw) == 0 {
		return nil, fmt.Errorf("%w: missing cr doc", ErrHQRemoteMalformedResponse)
	}
	var doc CRDoc
	if err := json.Unmarshal(docRaw, &doc); err != nil {
		return nil, fmt.Errorf("decode remote CR doc: %w", err)
	}
	if strings.TrimSpace(doc.UID) == "" {
		doc.UID = uid
	}
	localCR := crFromDoc(&doc)
	fingerprint := strings.TrimSpace(remoteCR.CRFingerprint)
	if fingerprint == "" {
		var fpErr error
		fingerprint, fpErr = fingerprintHQIntentCR(localCR)
		if fpErr != nil {
			return nil, fpErr
		}
	}
	return &HQCRDetail{
		UID:         strings.TrimSpace(doc.UID),
		Fingerprint: fingerprint,
		CR:          localCR,
	}, nil
}

func (s *Service) HQAddCRNote(uid, note string) (*model.HQPatchApplyResponse, error) {
	uid = strings.TrimSpace(uid)
	note = strings.TrimSpace(note)
	if uid == "" {
		return nil, fmt.Errorf("cr uid cannot be empty")
	}
	if note == "" {
		return nil, fmt.Errorf("note cannot be empty")
	}
	if err := s.ensureHQWritesAllowed(); err != nil {
		return nil, err
	}
	opPayload, err := json.Marshal(patchAddNoteOp{
		Op:   "add_note",
		Text: note,
	})
	if err != nil {
		return nil, err
	}
	return s.applyHQPatch(uid, []json.RawMessage{opPayload}, "hq note add")
}

func (s *Service) HQSetCRContract(uid string, patch ContractPatch) (*model.HQPatchApplyResponse, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, fmt.Errorf("cr uid cannot be empty")
	}
	if err := s.ensureHQWritesAllowed(); err != nil {
		return nil, err
	}

	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	changes := map[string]any{}
	if patch.Why != nil {
		changes["why"] = map[string]any{
			"after": strings.TrimSpace(*patch.Why),
		}
	}
	if patch.Scope != nil {
		scope, scopeErr := s.normalizeContractScopePrefixes(*patch.Scope)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if scopeErr := enforceScopeAllowlist(scope, policy.Scope.AllowedPrefixes, "hq contract scope"); scopeErr != nil {
			return nil, scopeErr
		}
		changes["scope"] = map[string]any{
			"after": scope,
		}
	}
	if patch.NonGoals != nil {
		changes["non_goals"] = map[string]any{
			"after": normalizeNonEmptyStringList(*patch.NonGoals),
		}
	}
	if patch.Invariants != nil {
		changes["invariants"] = map[string]any{
			"after": normalizeNonEmptyStringList(*patch.Invariants),
		}
	}
	if patch.BlastRadius != nil {
		changes["blast_radius"] = map[string]any{
			"after": strings.TrimSpace(*patch.BlastRadius),
		}
	}
	if patch.RiskCriticalScopes != nil {
		scopes, scopeErr := s.normalizeContractScopePrefixes(*patch.RiskCriticalScopes)
		if scopeErr != nil {
			return nil, scopeErr
		}
		changes["risk_critical_scopes"] = map[string]any{
			"after": scopes,
		}
	}
	if patch.RiskTierHint != nil {
		tierHint, hintErr := normalizeRiskTierHint(*patch.RiskTierHint)
		if hintErr != nil {
			return nil, hintErr
		}
		changes["risk_tier_hint"] = map[string]any{
			"after": tierHint,
		}
	}
	if patch.RiskRationale != nil {
		changes["risk_rationale"] = map[string]any{
			"after": strings.TrimSpace(*patch.RiskRationale),
		}
	}
	if patch.TestPlan != nil {
		changes["test_plan"] = map[string]any{
			"after": strings.TrimSpace(*patch.TestPlan),
		}
	}
	if patch.RollbackPlan != nil {
		changes["rollback_plan"] = map[string]any{
			"after": strings.TrimSpace(*patch.RollbackPlan),
		}
	}
	if len(changes) == 0 {
		return nil, ErrNoCRChanges
	}

	opPayload, err := json.Marshal(map[string]any{
		"op":      "set_contract",
		"changes": changes,
	})
	if err != nil {
		return nil, err
	}
	return s.applyHQPatch(uid, []json.RawMessage{opPayload}, "hq contract set")
}

func (s *Service) SyncCRFromHQ(uid string) (*HQSyncResult, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, fmt.Errorf("cr uid cannot be empty")
	}
	if err := s.ensureHQWritesAllowed(); err != nil {
		return nil, err
	}
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.RepoID) == "" {
		return nil, ErrHQRepoIDRequired
	}
	client := newHQClient(resolved.BaseURL, resolved.Token)
	remoteCR, err := client.GetCR(context.Background(), resolved.RepoID, uid)
	if err != nil {
		return nil, err
	}
	docRaw := remoteCR.Doc
	if len(docRaw) == 0 {
		docRaw = remoteCR.CR
	}
	if len(docRaw) == 0 {
		return nil, fmt.Errorf("%w: missing cr doc", ErrHQRemoteMalformedResponse)
	}
	var doc CRDoc
	if err := json.Unmarshal(docRaw, &doc); err != nil {
		return nil, fmt.Errorf("decode remote CR doc: %w", err)
	}
	if strings.TrimSpace(doc.UID) == "" {
		doc.UID = uid
	}
	imported := crFromDoc(&doc)
	if imported == nil {
		return nil, fmt.Errorf("unable to convert remote CR doc")
	}

	existing, existingErr := s.store.LoadCRByUID(strings.TrimSpace(imported.UID))
	created := false
	replaced := false
	switch {
	case existingErr == nil:
		imported.ID = existing.ID
		created = false
		replaced = true
	case existingErr != nil:
		if !errors.Is(existingErr, store.ErrNotFound) {
			return nil, existingErr
		}
		nextID, nextErr := s.store.NextCRID()
		if nextErr != nil {
			return nil, nextErr
		}
		imported.ID = nextID
		created = true
		replaced = false
	}

	if err := s.store.SaveCR(imported); err != nil {
		return nil, err
	}
	if err := s.syncCRRef(imported); err != nil {
		return nil, err
	}
	return &HQSyncResult{
		LocalCRID: imported.ID,
		CRUID:     strings.TrimSpace(imported.UID),
		Created:   created,
		Replaced:  replaced,
	}, nil
}

func (s *Service) applyHQPatch(uid string, ops []json.RawMessage, message string) (*model.HQPatchApplyResponse, error) {
	resolved, err := s.resolveHQRuntimeConfig()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resolved.RepoID) == "" {
		return nil, ErrHQRepoIDRequired
	}

	// Mutation should be drift-safe: patch must target the latest known fingerprint.
	client := newHQClient(resolved.BaseURL, resolved.Token)
	remoteCR, err := client.GetCR(context.Background(), resolved.RepoID, uid)
	if err != nil {
		return nil, err
	}
	fingerprint := strings.TrimSpace(remoteCR.CRFingerprint)
	if fingerprint == "" {
		docRaw := remoteCR.Doc
		if len(docRaw) == 0 {
			docRaw = remoteCR.CR
		}
		if len(docRaw) == 0 {
			return nil, fmt.Errorf("%w: missing cr doc", ErrHQRemoteMalformedResponse)
		}
		var doc CRDoc
		if err := json.Unmarshal(docRaw, &doc); err != nil {
			return nil, fmt.Errorf("decode remote CR doc: %w", err)
		}
		decodedCR := crFromDoc(&doc)
		if decodedCR == nil {
			return nil, fmt.Errorf("%w: invalid remote CR doc payload", ErrHQRemoteMalformedResponse)
		}
		var fpErr error
		fingerprint, fpErr = fingerprintHQIntentCR(decodedCR)
		if fpErr != nil {
			return nil, fpErr
		}
	}

	patch := model.CRPatch{
		SchemaVersion: patchSchemaV1,
		Target: model.CRPatchTarget{
			CRUID: uid,
		},
		Base: model.CRPatchBase{
			CRFingerprint: fingerprint,
		},
		Ops: ops,
		Meta: model.CRPatchMeta{
			Tool:    "sophia-cli",
			Message: strings.TrimSpace(message),
		},
	}
	return client.ApplyPatch(context.Background(), resolved.RepoID, uid, patch)
}

func (s *Service) ensureHQWritesAllowed() error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.MetadataMode) == model.MetadataModeTracked {
		return ErrHQTrackedModeBlocked
	}
	return nil
}

func normalizeHQRemoteAlias(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("hq remote alias cannot be empty")
	}
	return normalized, nil
}

func normalizeHQRepoID(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("hq repo id cannot be empty")
	}
	return normalized, nil
}

func normalizeHQBaseURL(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("hq base url cannot be empty")
	}
	parsed, err := url.Parse(normalized)
	if err != nil || !parsed.IsAbs() {
		return "", fmt.Errorf("invalid hq base url %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid hq base url %q: scheme must be http or https", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func (s *Service) resolveHQRuntimeConfig() (hqRuntimeConfig, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return hqRuntimeConfig{}, err
	}
	globalCfg, err := s.loadHQGlobalConfig()
	if err != nil {
		return hqRuntimeConfig{}, err
	}
	creds, err := s.loadHQCredentials()
	if err != nil {
		return hqRuntimeConfig{}, err
	}
	remoteAlias := strings.TrimSpace(cfg.HQRemote)
	if remoteAlias == "" {
		remoteAlias = strings.TrimSpace(globalCfg.HQRemote)
	}
	if remoteAlias == "" {
		remoteAlias = defaultHQRemoteAlias
	}
	baseURL := strings.TrimSpace(cfg.HQBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(globalCfg.HQBaseURL)
	}
	if baseURL == "" {
		baseURL = defaultHQBaseURL
	}
	baseURL, err = normalizeHQBaseURL(baseURL)
	if err != nil {
		return hqRuntimeConfig{}, err
	}
	repoID := strings.TrimSpace(cfg.HQRepoID)
	if repoID == "" {
		repoID = strings.TrimSpace(globalCfg.HQRepoID)
	}
	token := ""
	if creds.Tokens != nil {
		token = strings.TrimSpace(creds.Tokens[remoteAlias])
	}
	return hqRuntimeConfig{
		RemoteAlias:  remoteAlias,
		RepoID:       repoID,
		BaseURL:      baseURL,
		Token:        token,
		MetadataMode: strings.TrimSpace(cfg.MetadataMode),
	}, nil
}

func (s *Service) hqGlobalConfigPath() (string, error) {
	root, err := s.hqUserConfigRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hq.yaml"), nil
}

func (s *Service) hqCredentialPath() (string, error) {
	root, err := s.hqUserConfigRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "hq.credentials.yaml"), nil
}

func (s *Service) hqUserConfigRoot() (string, error) {
	xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "sophia"), nil
	}
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return "", fmt.Errorf("resolve user config dir: HOME is not set and XDG_CONFIG_HOME is empty")
	}
	return filepath.Join(home, ".config", "sophia"), nil
}

func (s *Service) loadHQGlobalConfig() (hqGlobalConfig, error) {
	path, err := s.hqGlobalConfigPath()
	if err != nil {
		return hqGlobalConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return hqGlobalConfig{Version: hqConfigVersionV1}, nil
		}
		return hqGlobalConfig{}, fmt.Errorf("read HQ config %q: %w", path, err)
	}
	var cfg hqGlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return hqGlobalConfig{}, fmt.Errorf("parse HQ config %q: %w", path, err)
	}
	if strings.TrimSpace(cfg.Version) == "" {
		cfg.Version = hqConfigVersionV1
	}
	return cfg, nil
}

func (s *Service) saveHQGlobalConfig(cfg hqGlobalConfig) error {
	path, err := s.hqGlobalConfigPath()
	if err != nil {
		return err
	}
	cfg.Version = hqConfigVersionV1
	return writeHQYAMLAtomic(path, cfg, 0o600)
}

func (s *Service) loadHQCredentials() (hqCredentialFile, error) {
	path, err := s.hqCredentialPath()
	if err != nil {
		return hqCredentialFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return hqCredentialFile{
				Version: hqConfigVersionV1,
				Tokens:  map[string]string{},
			}, nil
		}
		return hqCredentialFile{}, fmt.Errorf("read HQ credentials %q: %w", path, err)
	}
	var creds hqCredentialFile
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return hqCredentialFile{}, fmt.Errorf("parse HQ credentials %q: %w", path, err)
	}
	if strings.TrimSpace(creds.Version) == "" {
		creds.Version = hqConfigVersionV1
	}
	if creds.Tokens == nil {
		creds.Tokens = map[string]string{}
	}
	return creds, nil
}

func (s *Service) saveHQCredentials(creds hqCredentialFile) error {
	path, err := s.hqCredentialPath()
	if err != nil {
		return err
	}
	creds.Version = hqConfigVersionV1
	if creds.Tokens == nil {
		creds.Tokens = map[string]string{}
	}
	return writeHQYAMLAtomic(path, creds, 0o600)
}

func writeHQYAMLAtomic(path string, value any, perm os.FileMode) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for %q: %w", path, err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return fmt.Errorf("write %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}

type hqClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newHQClient(baseURL, token string) *hqClient {
	return &hqClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   strings.TrimSpace(token),
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *hqClient) ListCRs(ctx context.Context, repoID string) ([]model.HQCRSummary, error) {
	endpoint, err := c.urlFor(path.Join("api", "v1", "repos", url.PathEscape(strings.TrimSpace(repoID)), "crs"))
	if err != nil {
		return nil, err
	}
	var response model.HQListCRsResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	if len(response.Summaries) > 0 {
		return response.Summaries, nil
	}
	return response.Items, nil
}

func (c *hqClient) GetCR(ctx context.Context, repoID, uid string) (*model.HQGetCRResponse, error) {
	endpoint, err := c.urlFor(path.Join("api", "v1", "repos", url.PathEscape(strings.TrimSpace(repoID)), "crs", url.PathEscape(strings.TrimSpace(uid))))
	if err != nil {
		return nil, err
	}
	var response model.HQGetCRResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *hqClient) ApplyPatch(ctx context.Context, repoID, uid string, patch model.CRPatch) (*model.HQPatchApplyResponse, error) {
	endpoint, err := c.urlFor(path.Join("api", "v1", "repos", url.PathEscape(strings.TrimSpace(repoID)), "crs", url.PathEscape(strings.TrimSpace(uid)), "patch"))
	if err != nil {
		return nil, err
	}
	request := model.HQPatchApplyRequest{
		SchemaVersion: model.HQSchemaV1,
		Patch:         patch,
	}
	var response model.HQPatchApplyResponse
	if err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.CRUID) == "" {
		response.CRUID = strings.TrimSpace(uid)
	}
	return &response, nil
}

func (c *hqClient) urlFor(extraPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(c.baseURL))
	if err != nil || !parsed.IsAbs() {
		return "", fmt.Errorf("invalid hq base url %q", c.baseURL)
	}
	parsed.Path = path.Join(parsed.Path, extraPath)
	return parsed.String(), nil
}

func (c *hqClient) doJSON(ctx context.Context, method, endpoint string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.token))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return &HQRemoteError{
			StatusCode: resp.StatusCode,
			Method:     method,
			URL:        endpoint,
			Message:    strings.TrimSpace(string(payload)),
		}
	}
	if responseBody == nil || len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, responseBody); err != nil {
		return fmt.Errorf("decode HQ response: %w", err)
	}
	return nil
}
