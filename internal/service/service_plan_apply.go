package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ApplyCRPlanOptions struct {
	FilePath string
	DryRun   bool
	KeepFile bool
}

type ApplyCRPlanCreatedCR struct {
	Key        string
	ID         int
	UID        string
	Branch     string
	ParentCRID int
}

type ApplyCRPlanCreatedTask struct {
	CRKey   string
	TaskKey string
	TaskID  int
}

type ApplyCRPlanDelegation struct {
	ParentCRKey   string
	ParentTaskKey string
	ChildCRKey    string
	ChildTaskID   int
}

type ApplyCRPlanResult struct {
	FilePath          string
	DryRun            bool
	Consumed          bool
	PlannedOperations []string
	CreatedCRs        []ApplyCRPlanCreatedCR
	CreatedTasks      []ApplyCRPlanCreatedTask
	Delegations       []ApplyCRPlanDelegation
	Warnings          []string
}

type crPlanSpec struct {
	Version string         `yaml:"version"`
	CRs     []crPlanCRSpec `yaml:"crs"`
}

type crPlanCRSpec struct {
	Key         string           `yaml:"key"`
	Title       string           `yaml:"title"`
	Description string           `yaml:"description"`
	Base        string           `yaml:"base,omitempty"`
	ParentKey   string           `yaml:"parent_key,omitempty"`
	BranchAlias string           `yaml:"branch_alias,omitempty"`
	OwnerPrefix string           `yaml:"owner_prefix,omitempty"`
	Contract    crPlanContract   `yaml:"contract,omitempty"`
	Tasks       []crPlanTaskSpec `yaml:"tasks,omitempty"`
}

type crPlanTaskSpec struct {
	Key        string             `yaml:"key"`
	Title      string             `yaml:"title"`
	Contract   crPlanTaskContract `yaml:"contract,omitempty"`
	DelegateTo []string           `yaml:"delegate_to,omitempty"`
}

type crPlanContract struct {
	Why                string   `yaml:"why,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	NonGoals           []string `yaml:"non_goals,omitempty"`
	Invariants         []string `yaml:"invariants,omitempty"`
	BlastRadius        string   `yaml:"blast_radius,omitempty"`
	RiskCriticalScopes []string `yaml:"risk_critical_scopes,omitempty"`
	RiskTierHint       string   `yaml:"risk_tier_hint,omitempty"`
	RiskRationale      string   `yaml:"risk_rationale,omitempty"`
	TestPlan           string   `yaml:"test_plan,omitempty"`
	RollbackPlan       string   `yaml:"rollback_plan,omitempty"`
}

type crPlanTaskContract struct {
	Intent             string   `yaml:"intent,omitempty"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	AcceptanceChecks   []string `yaml:"acceptance_checks,omitempty"`
}

type planTaskRef struct {
	CRKey   string
	TaskKey string
}

type planOrder struct {
	CROrder      []string
	ByKey        map[string]crPlanCRSpec
	Delegations  []ApplyCRPlanDelegation
	TaskOrderMap map[planTaskRef]int
}

type planCRPrediction struct {
	ID         int
	UID        string
	Branch     string
	ParentCRID int
}

func (s *Service) ApplyCRPlan(opts ApplyCRPlanOptions) (*ApplyCRPlanResult, error) {
	planPath := strings.TrimSpace(opts.FilePath)
	if planPath == "" {
		return nil, fmt.Errorf("--file is required")
	}
	if err := s.store.EnsureInitialized(); err != nil {
		return nil, err
	}
	if err := s.ensureNoMergeInProgressInCurrentWorktree(); err != nil {
		return nil, err
	}

	plan, err := readCRPlanFile(planPath)
	if err != nil {
		return nil, err
	}
	order, err := s.validateCRPlan(plan)
	if err != nil {
		return nil, err
	}

	result := &ApplyCRPlanResult{
		FilePath:          planPath,
		DryRun:            opts.DryRun,
		Consumed:          false,
		PlannedOperations: s.planOperations(order),
		CreatedCRs:        []ApplyCRPlanCreatedCR{},
		CreatedTasks:      []ApplyCRPlanCreatedTask{},
		Delegations:       []ApplyCRPlanDelegation{},
		Warnings:          []string{},
	}

	startBranch, _ := s.git.CurrentBranch()
	startBranch = strings.TrimSpace(startBranch)
	restoreBranch := func() string {
		if startBranch == "" {
			return ""
		}
		if !s.git.BranchExists(startBranch) {
			return fmt.Sprintf("starting branch %q no longer exists; unable to restore", startBranch)
		}
		if checkoutErr := s.git.CheckoutBranch(startBranch); checkoutErr != nil {
			return fmt.Sprintf("failed to restore starting branch %q: %v", startBranch, checkoutErr)
		}
		return ""
	}

	if opts.DryRun {
		if err := s.populateDryRunPredictions(result, order); err != nil {
			return nil, err
		}
		if warning := restoreBranch(); warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		return result, nil
	}

	applied, err := s.applyCRPlan(plan, order)
	if warning := restoreBranch(); warning != "" {
		if applied != nil {
			applied.Warnings = append(applied.Warnings, warning)
		}
	}
	if err != nil {
		return nil, err
	}

	result.CreatedCRs = applied.CreatedCRs
	result.CreatedTasks = applied.CreatedTasks
	result.Delegations = applied.Delegations
	result.Warnings = append(result.Warnings, applied.Warnings...)

	if !opts.KeepFile {
		if removeErr := os.Remove(planPath); removeErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("apply succeeded but failed to consume plan file %q: %v", planPath, removeErr))
		} else {
			result.Consumed = true
		}
	}

	return result, nil
}

func readCRPlanFile(path string) (*crPlanSpec, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return nil, fmt.Errorf("invalid plan file path %q", path)
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("read plan file %q: %w", cleanPath, err)
	}

	var plan crPlanSpec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&plan); err != nil {
		return nil, fmt.Errorf("parse plan file %q: %w", cleanPath, err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, fmt.Errorf("parse plan file %q: multiple YAML documents are not supported", cleanPath)
	} else if err != io.EOF {
		return nil, fmt.Errorf("parse plan file %q: %w", cleanPath, err)
	}
	return &plan, nil
}

func (s *Service) validateCRPlan(plan *crPlanSpec) (*planOrder, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is required")
	}
	if strings.TrimSpace(plan.Version) != "v1" {
		return nil, fmt.Errorf("invalid plan version %q (expected v1)", strings.TrimSpace(plan.Version))
	}
	if len(plan.CRs) == 0 {
		return nil, fmt.Errorf("plan must include at least one CR")
	}
	policy, err := s.repoPolicy()
	if err != nil {
		return nil, err
	}
	allowedScopePrefixes := append([]string(nil), policy.Scope.AllowedPrefixes...)
	byKey, keyOrder, err := s.normalizeAndValidatePlanCRSpecs(plan.CRs, allowedScopePrefixes)
	if err != nil {
		return nil, err
	}
	if err := s.validatePlanBaseRefs(byKey, keyOrder); err != nil {
		return nil, err
	}
	if err := validateCRPlanAcyclic(byKey, keyOrder); err != nil {
		return nil, err
	}

	crOrder, err := planTopologicalOrder(byKey, keyOrder)
	if err != nil {
		return nil, err
	}
	delegations, taskOrderMap, err := buildPlanDelegationsAndTaskOrder(crOrder, byKey)
	if err != nil {
		return nil, err
	}

	return &planOrder{
		CROrder:      crOrder,
		ByKey:        byKey,
		Delegations:  delegations,
		TaskOrderMap: taskOrderMap,
	}, nil
}

func (s *Service) normalizeAndValidatePlanCRSpecs(rawCRs []crPlanCRSpec, allowedScopePrefixes []string) (map[string]crPlanCRSpec, []string, error) {
	byKey := make(map[string]crPlanCRSpec, len(rawCRs))
	keyOrder := make([]string, 0, len(rawCRs))
	for i, raw := range rawCRs {
		cr, err := s.normalizeAndValidatePlanCRSpec(i, raw, allowedScopePrefixes, byKey)
		if err != nil {
			return nil, nil, err
		}
		byKey[cr.Key] = cr
		keyOrder = append(keyOrder, cr.Key)
	}
	return byKey, keyOrder, nil
}

func (s *Service) normalizeAndValidatePlanCRSpec(index int, raw crPlanCRSpec, allowedScopePrefixes []string, existing map[string]crPlanCRSpec) (crPlanCRSpec, error) {
	cr := raw
	cr.Key = strings.TrimSpace(cr.Key)
	cr.Title = strings.TrimSpace(cr.Title)
	cr.Description = strings.TrimSpace(cr.Description)
	cr.Base = strings.TrimSpace(cr.Base)
	cr.ParentKey = strings.TrimSpace(cr.ParentKey)
	cr.BranchAlias = strings.TrimSpace(cr.BranchAlias)
	cr.OwnerPrefix = strings.TrimSpace(cr.OwnerPrefix)
	if cr.Key == "" {
		return crPlanCRSpec{}, fmt.Errorf("cr[%d] key cannot be empty", index)
	}
	if _, exists := existing[cr.Key]; exists {
		return crPlanCRSpec{}, fmt.Errorf("duplicate cr key %q", cr.Key)
	}
	if cr.Title == "" {
		return crPlanCRSpec{}, fmt.Errorf("cr[%d] (%s) title cannot be empty", index, cr.Key)
	}
	if cr.Base != "" && cr.ParentKey != "" {
		return crPlanCRSpec{}, fmt.Errorf("cr %q cannot define both base and parent_key", cr.Key)
	}
	if cr.BranchAlias != "" && cr.OwnerPrefix != "" {
		return crPlanCRSpec{}, fmt.Errorf("cr %q cannot define both branch_alias and owner_prefix", cr.Key)
	}
	if cr.BranchAlias != "" {
		if _, err := validateCRBranchAliasShape(cr.BranchAlias); err != nil {
			return crPlanCRSpec{}, fmt.Errorf("cr %q branch_alias invalid: %v", cr.Key, err)
		}
	}
	if cr.OwnerPrefix != "" {
		if _, err := normalizeCRBranchOwnerPrefix(cr.OwnerPrefix); err != nil {
			return crPlanCRSpec{}, fmt.Errorf("cr %q owner_prefix invalid: %v", cr.Key, err)
		}
	}
	if err := s.validatePlanContract(cr.Key, cr.Contract, allowedScopePrefixes); err != nil {
		return crPlanCRSpec{}, err
	}
	tasks, err := s.normalizeAndValidatePlanTasks(cr.Key, cr.Tasks, allowedScopePrefixes)
	if err != nil {
		return crPlanCRSpec{}, err
	}
	cr.Tasks = tasks
	return cr, nil
}

func (s *Service) normalizeAndValidatePlanTasks(crKey string, tasks []crPlanTaskSpec, allowedScopePrefixes []string) ([]crPlanTaskSpec, error) {
	taskKeys := map[string]struct{}{}
	normalized := make([]crPlanTaskSpec, 0, len(tasks))
	for i, raw := range tasks {
		task := raw
		task.Key = strings.TrimSpace(task.Key)
		task.Title = strings.TrimSpace(task.Title)
		if task.Key == "" {
			return nil, fmt.Errorf("cr %q task[%d] key cannot be empty", crKey, i)
		}
		if _, exists := taskKeys[task.Key]; exists {
			return nil, fmt.Errorf("cr %q contains duplicate task key %q", crKey, task.Key)
		}
		taskKeys[task.Key] = struct{}{}
		if task.Title == "" {
			return nil, fmt.Errorf("cr %q task %q title cannot be empty", crKey, task.Key)
		}
		if err := s.validatePlanTaskContract(crKey, task.Key, task.Contract, allowedScopePrefixes); err != nil {
			return nil, err
		}
		seenDelegates := map[string]struct{}{}
		for _, childRaw := range task.DelegateTo {
			childKey := strings.TrimSpace(childRaw)
			if childKey == "" {
				return nil, fmt.Errorf("cr %q task %q delegate_to cannot contain empty key", crKey, task.Key)
			}
			if _, exists := seenDelegates[childKey]; exists {
				return nil, fmt.Errorf("cr %q task %q duplicate delegate_to key %q", crKey, task.Key, childKey)
			}
			seenDelegates[childKey] = struct{}{}
		}
		normalized = append(normalized, task)
	}
	return normalized, nil
}

func (s *Service) validatePlanBaseRefs(byKey map[string]crPlanCRSpec, keyOrder []string) error {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return err
	}
	for _, key := range keyOrder {
		cr := byKey[key]
		if cr.ParentKey != "" {
			if _, exists := byKey[cr.ParentKey]; !exists {
				return fmt.Errorf("cr %q parent_key %q not found", cr.Key, cr.ParentKey)
			}
			continue
		}
		effectiveRef := cr.Base
		if effectiveRef == "" {
			effectiveRef = cfg.BaseBranch
		}
		_, resolveErr := s.git.ResolveRef(effectiveRef)
		if resolveErr == nil {
			continue
		}
		if !s.git.HasCommit() && (effectiveRef == cfg.BaseBranch || s.git.BranchExists(effectiveRef)) {
			continue
		}
		return fmt.Errorf("cr %q base ref %q is invalid: %w", cr.Key, effectiveRef, resolveErr)
	}
	return nil
}

func buildPlanDelegationsAndTaskOrder(crOrder []string, byKey map[string]crPlanCRSpec) ([]ApplyCRPlanDelegation, map[planTaskRef]int, error) {
	delegations := make([]ApplyCRPlanDelegation, 0)
	taskOrderMap := map[planTaskRef]int{}
	taskOrderIndex := 0
	for _, crKey := range crOrder {
		cr := byKey[crKey]
		for _, task := range cr.Tasks {
			ref := planTaskRef{CRKey: crKey, TaskKey: task.Key}
			taskOrderMap[ref] = taskOrderIndex
			taskOrderIndex++
			for _, childRaw := range task.DelegateTo {
				childKey := strings.TrimSpace(childRaw)
				childCR, ok := byKey[childKey]
				if !ok {
					return nil, nil, fmt.Errorf("cr %q task %q delegate_to child %q not found", crKey, task.Key, childKey)
				}
				if childCR.ParentKey != crKey {
					return nil, nil, fmt.Errorf("cr %q task %q can only delegate to direct child CRs, but %q parent is %q", crKey, task.Key, childKey, childCR.ParentKey)
				}
				delegations = append(delegations, ApplyCRPlanDelegation{
					ParentCRKey:   crKey,
					ParentTaskKey: task.Key,
					ChildCRKey:    childKey,
				})
			}
		}
	}
	return delegations, taskOrderMap, nil
}

func (s *Service) validatePlanContract(crKey string, contract crPlanContract, allowedScopePrefixes []string) error {
	if len(contract.Scope) > 0 {
		scope, err := s.normalizeContractScopePrefixes(contract.Scope)
		if err != nil {
			return fmt.Errorf("cr %q contract scope invalid: %w", crKey, err)
		}
		if err := enforceScopeAllowlist(scope, allowedScopePrefixes, fmt.Sprintf("cr %q contract scope", crKey)); err != nil {
			return err
		}
	}
	if len(contract.RiskCriticalScopes) > 0 {
		if _, err := s.normalizeContractScopePrefixes(contract.RiskCriticalScopes); err != nil {
			return fmt.Errorf("cr %q contract risk_critical_scopes invalid: %w", crKey, err)
		}
	}
	if _, err := normalizeRiskTierHint(contract.RiskTierHint); err != nil {
		return fmt.Errorf("cr %q contract risk_tier_hint invalid: %w", crKey, err)
	}
	return nil
}

func (s *Service) validatePlanTaskContract(crKey, taskKey string, contract crPlanTaskContract, allowedScopePrefixes []string) error {
	if len(contract.Scope) > 0 {
		scope, err := s.normalizeContractScopePrefixes(contract.Scope)
		if err != nil {
			return fmt.Errorf("cr %q task %q contract scope invalid: %w", crKey, taskKey, err)
		}
		if err := enforceScopeAllowlist(scope, allowedScopePrefixes, fmt.Sprintf("cr %q task %q contract scope", crKey, taskKey)); err != nil {
			return err
		}
	}
	return nil
}

func validateCRPlanAcyclic(byKey map[string]crPlanCRSpec, keyOrder []string) error {
	state := map[string]int{}
	var visit func(string) error
	visit = func(key string) error {
		switch state[key] {
		case 1:
			return fmt.Errorf("plan parent graph contains a cycle involving %q", key)
		case 2:
			return nil
		}
		state[key] = 1
		parent := strings.TrimSpace(byKey[key].ParentKey)
		if parent != "" {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[key] = 2
		return nil
	}
	for _, key := range keyOrder {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}

func planTopologicalOrder(byKey map[string]crPlanCRSpec, keyOrder []string) ([]string, error) {
	children := map[string][]string{}
	inDegree := map[string]int{}
	for _, key := range keyOrder {
		inDegree[key] = 0
	}
	for _, key := range keyOrder {
		parent := strings.TrimSpace(byKey[key].ParentKey)
		if parent == "" {
			continue
		}
		children[parent] = append(children[parent], key)
		inDegree[key]++
	}

	queue := make([]string, 0, len(keyOrder))
	for _, key := range keyOrder {
		if inDegree[key] == 0 {
			queue = append(queue, key)
		}
	}

	order := make([]string, 0, len(keyOrder))
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		order = append(order, key)
		for _, child := range children[key] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(order) != len(keyOrder) {
		return nil, fmt.Errorf("unable to topologically order plan CRs")
	}
	return order, nil
}

func (s *Service) planOperations(order *planOrder) []string {
	ops := make([]string, 0)
	for _, key := range order.CROrder {
		cr := order.ByKey[key]
		scopeRef := ""
		if cr.ParentKey != "" {
			scopeRef = fmt.Sprintf("parent_key=%s", cr.ParentKey)
		} else if cr.Base != "" {
			scopeRef = fmt.Sprintf("base=%s", cr.Base)
		} else {
			scopeRef = "base=<default>"
		}
		ops = append(ops, fmt.Sprintf("cr.add key=%s title=%q %s", key, cr.Title, scopeRef))
		if hasPlanContract(cr.Contract) {
			ops = append(ops, fmt.Sprintf("cr.contract.set key=%s", key))
		}
		for _, task := range cr.Tasks {
			ops = append(ops, fmt.Sprintf("cr.task.add cr_key=%s task_key=%s title=%q", key, task.Key, task.Title))
			if hasPlanTaskContract(task.Contract) {
				ops = append(ops, fmt.Sprintf("cr.task.contract.set cr_key=%s task_key=%s", key, task.Key))
			}
		}
	}
	for _, delegation := range order.Delegations {
		ops = append(ops, fmt.Sprintf("cr.task.delegate parent_cr_key=%s parent_task_key=%s child_cr_key=%s", delegation.ParentCRKey, delegation.ParentTaskKey, delegation.ChildCRKey))
	}
	return ops
}

func (s *Service) populateDryRunPredictions(result *ApplyCRPlanResult, order *planOrder) error {
	predictedCRs, warnings, err := s.predictPlanCRs(order)
	if err != nil {
		return err
	}
	result.Warnings = append(result.Warnings, warnings...)
	nextTaskIDByCRKey := map[string]int{}

	for _, key := range order.CROrder {
		cr := order.ByKey[key]
		predicted, ok := predictedCRs[key]
		if !ok {
			return fmt.Errorf("predicted CR metadata for key %q is missing", key)
		}
		result.CreatedCRs = append(result.CreatedCRs, ApplyCRPlanCreatedCR{
			Key:        key,
			ID:         predicted.ID,
			UID:        predicted.UID,
			Branch:     predicted.Branch,
			ParentCRID: predicted.ParentCRID,
		})

		nextTaskID := 1
		for _, task := range cr.Tasks {
			result.CreatedTasks = append(result.CreatedTasks, ApplyCRPlanCreatedTask{
				CRKey:   key,
				TaskKey: task.Key,
				TaskID:  nextTaskID,
			})
			nextTaskID++
		}
		nextTaskIDByCRKey[key] = nextTaskID
	}

	for _, delegation := range order.Delegations {
		childTaskID := nextTaskIDByCRKey[delegation.ChildCRKey]
		nextTaskIDByCRKey[delegation.ChildCRKey] = childTaskID + 1
		result.Delegations = append(result.Delegations, ApplyCRPlanDelegation{
			ParentCRKey:   delegation.ParentCRKey,
			ParentTaskKey: delegation.ParentTaskKey,
			ChildCRKey:    delegation.ChildCRKey,
			ChildTaskID:   childTaskID,
		})
	}

	return nil
}

func (s *Service) predictPlanCRs(order *planOrder) (map[string]planCRPrediction, []string, error) {
	idx, err := s.store.LoadIndex()
	if err != nil {
		return nil, nil, err
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	existingCRs, err := s.store.ListCRs()
	if err != nil {
		return nil, nil, err
	}
	maxID := 0
	usedUIDs := map[string]struct{}{}
	warnings := []string{}
	for i := range existingCRs {
		if existingCRs[i].ID > maxID {
			maxID = existingCRs[i].ID
		}
		uid := strings.TrimSpace(existingCRs[i].UID)
		if uid == "" {
			continue
		}
		normalizedUID, normalizeErr := normalizeCRUID(uid)
		if normalizeErr != nil {
			warnings = append(warnings, fmt.Sprintf("ignoring malformed existing CR uid %q on CR %d during plan prediction", uid, existingCRs[i].ID))
			continue
		}
		usedUIDs[normalizedUID] = struct{}{}
	}
	branches, err := s.git.LocalBranches("")
	if err == nil {
		for _, branch := range branches {
			if id, ok := parseCRBranchID(branch); ok && id > maxID {
				maxID = id
			}
		}
	}
	if strings.TrimSpace(cfg.BaseBranch) != "" {
		commits, recentErr := s.git.RecentCommits(cfg.BaseBranch, 5000)
		if recentErr == nil {
			for _, commit := range commits {
				if id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body); ok && id > maxID {
					maxID = id
				}
			}
		}
	}
	required := maxID + 1
	if required < 1 {
		required = 1
	}
	nextCRID := idx.NextID
	if nextCRID < required {
		nextCRID = required
	}
	crIDByKey := map[string]int{}
	predictedBranches := map[string]struct{}{}
	predictions := map[string]planCRPrediction{}
	for _, key := range order.CROrder {
		cr := order.ByKey[key]
		parentID := 0
		if cr.ParentKey != "" {
			parentID = crIDByKey[cr.ParentKey]
		}
		uid, uidErr := allocatePlanCRUID(nextCRID, key, cr.Title, usedUIDs)
		if uidErr != nil {
			return nil, warnings, uidErr
		}

		branch := ""
		if strings.TrimSpace(cr.BranchAlias) != "" {
			alias, aliasErr := validateExplicitCRBranchAlias(cr.BranchAlias, nextCRID)
			if aliasErr != nil {
				return nil, warnings, aliasErr
			}
			if _, exists := predictedBranches[alias]; exists || s.git.BranchExists(alias) {
				return nil, warnings, fmt.Errorf("branch %q already exists", alias)
			}
			branch = alias
		} else {
			ownerPrefix := cfg.BranchOwnerPrefix
			if cr.OwnerPrefix != "" {
				ownerPrefix = cr.OwnerPrefix
			}
			alias, branchErr := formatCRBranchAliasWithFallback(cr.Title, ownerPrefix, uid, func(candidate string) bool {
				if _, exists := predictedBranches[candidate]; exists {
					return true
				}
				return s.git.BranchExists(candidate)
			})
			if branchErr != nil {
				return nil, warnings, branchErr
			}
			branch = alias
		}
		predictedBranches[branch] = struct{}{}
		predictions[key] = planCRPrediction{
			ID:         nextCRID,
			UID:        uid,
			Branch:     branch,
			ParentCRID: parentID,
		}
		crIDByKey[key] = nextCRID
		nextCRID++
	}
	return predictions, warnings, nil
}

func allocatePlanCRUID(nextCRID int, key, title string, usedUIDs map[string]struct{}) (string, error) {
	baseSeed := fmt.Sprintf("plan-v1|id=%d|key=%s|title=%s", nextCRID, strings.TrimSpace(strings.ToLower(key)), strings.TrimSpace(strings.ToLower(title)))
	for attempt := 0; attempt < 32; attempt++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|attempt=%d", baseSeed, attempt)))
		raw := sum[:16]
		// Keep RFC 4122 variant/version bits for compatibility with UUID tooling.
		raw[6] = (raw[6] & 0x0f) | 0x40
		raw[8] = (raw[8] & 0x3f) | 0x80
		uid, err := normalizeCRUID(fmt.Sprintf("cr_%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]))
		if err != nil {
			return "", err
		}
		if _, exists := usedUIDs[uid]; exists {
			continue
		}
		usedUIDs[uid] = struct{}{}
		return uid, nil
	}
	return "", fmt.Errorf("unable to allocate deterministic CR uid for key %q", key)
}

func (s *Service) applyCRPlan(plan *crPlanSpec, order *planOrder) (*ApplyCRPlanResult, error) {
	result := &ApplyCRPlanResult{
		CreatedCRs:   []ApplyCRPlanCreatedCR{},
		CreatedTasks: []ApplyCRPlanCreatedTask{},
		Delegations:  []ApplyCRPlanDelegation{},
		Warnings:     []string{},
	}
	predictedCRs, warnings, err := s.predictPlanCRs(order)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	crIDByKey := map[string]int{}
	taskIDByRef := map[planTaskRef]int{}
	for _, key := range order.CROrder {
		crSpec := order.ByKey[key]
		addOpts := AddCROptions{}
		if crSpec.ParentKey != "" {
			addOpts.ParentCRID = crIDByKey[crSpec.ParentKey]
		} else if crSpec.Base != "" {
			addOpts.BaseRef = crSpec.Base
		}
		addOpts.BranchAlias = crSpec.BranchAlias
		if crSpec.OwnerPrefix != "" {
			addOpts.OwnerPrefix = crSpec.OwnerPrefix
			addOpts.OwnerPrefixSet = true
		}
		if predicted, ok := predictedCRs[key]; ok {
			addOpts.UIDOverride = predicted.UID
		}
		createdCR, warnings, err := s.AddCRWithOptionsWithWarnings(crSpec.Title, crSpec.Description, addOpts)
		if err != nil {
			return nil, fmt.Errorf("create cr %q: %w", key, err)
		}
		crIDByKey[key] = createdCR.ID
		result.CreatedCRs = append(result.CreatedCRs, ApplyCRPlanCreatedCR{
			Key:        key,
			ID:         createdCR.ID,
			UID:        strings.TrimSpace(createdCR.UID),
			Branch:     createdCR.Branch,
			ParentCRID: createdCR.ParentCRID,
		})
		for _, warning := range warnings {
			result.Warnings = append(result.Warnings, fmt.Sprintf("cr %q: %s", key, warning))
		}

		if hasPlanContract(crSpec.Contract) {
			patch := contractPatchFromPlan(crSpec.Contract)
			if _, err := s.SetCRContract(createdCR.ID, patch); err != nil {
				return nil, fmt.Errorf("set contract for cr %q: %w", key, err)
			}
		}

		for _, taskSpec := range crSpec.Tasks {
			createdTask, err := s.AddTask(createdCR.ID, taskSpec.Title)
			if err != nil {
				return nil, fmt.Errorf("add task %q for cr %q: %w", taskSpec.Key, key, err)
			}
			ref := planTaskRef{CRKey: key, TaskKey: taskSpec.Key}
			taskIDByRef[ref] = createdTask.ID
			result.CreatedTasks = append(result.CreatedTasks, ApplyCRPlanCreatedTask{
				CRKey:   key,
				TaskKey: taskSpec.Key,
				TaskID:  createdTask.ID,
			})
			if hasPlanTaskContract(taskSpec.Contract) {
				patch := taskContractPatchFromPlan(taskSpec.Contract)
				if _, err := s.SetTaskContract(createdCR.ID, createdTask.ID, patch); err != nil {
					return nil, fmt.Errorf("set contract for cr %q task %q: %w", key, taskSpec.Key, err)
				}
			}
		}
	}

	for _, delegation := range order.Delegations {
		parentCRID := crIDByKey[delegation.ParentCRKey]
		parentTaskID := taskIDByRef[planTaskRef{CRKey: delegation.ParentCRKey, TaskKey: delegation.ParentTaskKey}]
		childCRID := crIDByKey[delegation.ChildCRKey]
		delegated, err := s.DelegateTaskToChild(parentCRID, parentTaskID, childCRID)
		if err != nil {
			return nil, fmt.Errorf("delegate task %q/%q to child %q: %w", delegation.ParentCRKey, delegation.ParentTaskKey, delegation.ChildCRKey, err)
		}
		result.Delegations = append(result.Delegations, ApplyCRPlanDelegation{
			ParentCRKey:   delegation.ParentCRKey,
			ParentTaskKey: delegation.ParentTaskKey,
			ChildCRKey:    delegation.ChildCRKey,
			ChildTaskID:   delegated.ChildTaskID,
		})
	}

	return result, nil
}

func hasPlanContract(contract crPlanContract) bool {
	if strings.TrimSpace(contract.Why) != "" || strings.TrimSpace(contract.BlastRadius) != "" || strings.TrimSpace(contract.RiskTierHint) != "" || strings.TrimSpace(contract.RiskRationale) != "" || strings.TrimSpace(contract.TestPlan) != "" || strings.TrimSpace(contract.RollbackPlan) != "" {
		return true
	}
	return len(contract.Scope) > 0 || len(contract.NonGoals) > 0 || len(contract.Invariants) > 0 || len(contract.RiskCriticalScopes) > 0
}

func hasPlanTaskContract(contract crPlanTaskContract) bool {
	if strings.TrimSpace(contract.Intent) != "" {
		return true
	}
	return len(contract.AcceptanceCriteria) > 0 || len(contract.Scope) > 0 || len(contract.AcceptanceChecks) > 0
}

func contractPatchFromPlan(contract crPlanContract) ContractPatch {
	why := strings.TrimSpace(contract.Why)
	scope := append([]string(nil), contract.Scope...)
	nonGoals := append([]string(nil), contract.NonGoals...)
	invariants := append([]string(nil), contract.Invariants...)
	blastRadius := strings.TrimSpace(contract.BlastRadius)
	riskCriticalScopes := append([]string(nil), contract.RiskCriticalScopes...)
	riskTierHint := strings.TrimSpace(contract.RiskTierHint)
	riskRationale := strings.TrimSpace(contract.RiskRationale)
	testPlan := strings.TrimSpace(contract.TestPlan)
	rollbackPlan := strings.TrimSpace(contract.RollbackPlan)
	return ContractPatch{
		Why:                &why,
		Scope:              &scope,
		NonGoals:           &nonGoals,
		Invariants:         &invariants,
		BlastRadius:        &blastRadius,
		RiskCriticalScopes: &riskCriticalScopes,
		RiskTierHint:       &riskTierHint,
		RiskRationale:      &riskRationale,
		TestPlan:           &testPlan,
		RollbackPlan:       &rollbackPlan,
	}
}

func taskContractPatchFromPlan(contract crPlanTaskContract) TaskContractPatch {
	intent := strings.TrimSpace(contract.Intent)
	acceptance := append([]string(nil), contract.AcceptanceCriteria...)
	scope := append([]string(nil), contract.Scope...)
	acceptanceChecks := append([]string(nil), contract.AcceptanceChecks...)
	return TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
		AcceptanceChecks:   &acceptanceChecks,
	}
}
