package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

type fakeCallCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *fakeCallCounter) hit(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[name]++
}

func (c *fakeCallCounter) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[name]
}

type fakeCRStore struct {
	counter fakeCallCounter

	mu sync.Mutex

	initialized bool
	config      model.Config
	index       model.Index
	crs         map[int]*model.CR
	sophiaDir   string

	ensureInitErr error
	loadConfigErr error
	nextIDErr     error
	loadIndexErr  error
	saveIndexErr  error
	loadCRErr     error
	saveCRErr     error
	listErr       error
	loadByIDErr   map[int]error
	saveHook      func(*model.CR) error
}

func newFakeCRStore() *fakeCRStore {
	return &fakeCRStore{
		initialized: true,
		config: model.Config{
			Version:      "v0",
			BaseBranch:   "main",
			MetadataMode: model.MetadataModeLocal,
		},
		index: model.Index{NextID: 1},
		crs:   map[int]*model.CR{},
	}
}

func (s *fakeCRStore) Calls(name string) int {
	return s.counter.count(name)
}

func (s *fakeCRStore) EnsureInitialized() error {
	s.counter.hit("EnsureInitialized")
	if s.ensureInitErr != nil {
		return s.ensureInitErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return fmt.Errorf("not initialized")
	}
	return nil
}

func (s *fakeCRStore) LoadConfig() (model.Config, error) {
	s.counter.hit("LoadConfig")
	if s.loadConfigErr != nil {
		return model.Config{}, s.loadConfigErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config, nil
}

func (s *fakeCRStore) NextCRID() (int, error) {
	s.counter.hit("NextCRID")
	if s.nextIDErr != nil {
		return 0, s.nextIDErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index.NextID < 1 {
		s.index.NextID = 1
	}
	id := s.index.NextID
	s.index.NextID++
	return id, nil
}

func (s *fakeCRStore) LoadCR(id int) (*model.CR, error) {
	s.counter.hit("LoadCR")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadCRErr != nil {
		return nil, s.loadCRErr
	}
	if err, ok := s.loadByIDErr[id]; ok {
		return nil, err
	}
	cr, ok := s.crs[id]
	if !ok {
		return nil, fmt.Errorf("cr %d not found", id)
	}
	return cloneHarnessCR(cr), nil
}

func (s *fakeCRStore) SaveCR(cr *model.CR) error {
	s.counter.hit("SaveCR")
	if s.saveCRErr != nil {
		return s.saveCRErr
	}
	if cr == nil {
		return fmt.Errorf("cr cannot be nil")
	}
	copyCR := cloneHarnessCR(cr)
	if s.saveHook != nil {
		if err := s.saveHook(copyCR); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.crs == nil {
		s.crs = map[int]*model.CR{}
	}
	s.crs[copyCR.ID] = copyCR
	if copyCR.ID >= s.index.NextID {
		s.index.NextID = copyCR.ID + 1
	}
	return nil
}

func (s *fakeCRStore) ListCRs() ([]model.CR, error) {
	s.counter.hit("ListCRs")
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int, 0, len(s.crs))
	for id := range s.crs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]model.CR, 0, len(ids))
	for _, id := range ids {
		copyCR := cloneHarnessCR(s.crs[id])
		out = append(out, *copyCR)
	}
	return out, nil
}

func (s *fakeCRStore) LoadIndex() (model.Index, error) {
	s.counter.hit("LoadIndex")
	if s.loadIndexErr != nil {
		return model.Index{}, s.loadIndexErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index.NextID < 1 {
		s.index.NextID = 1
	}
	return s.index, nil
}

func (s *fakeCRStore) SaveIndex(idx model.Index) error {
	s.counter.hit("SaveIndex")
	if s.saveIndexErr != nil {
		return s.saveIndexErr
	}
	if idx.NextID < 1 {
		idx.NextID = 1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index = idx
	return nil
}

func (s *fakeCRStore) SophiaDir() string {
	s.counter.hit("SophiaDir")
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.sophiaDir)
}

func (s *fakeCRStore) SeedCR(cr *model.CR) {
	if cr == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.crs == nil {
		s.crs = map[int]*model.CR{}
	}
	s.crs[cr.ID] = cloneHarnessCR(cr)
	if cr.ID >= s.index.NextID {
		s.index.NextID = cr.ID + 1
	}
}

type fakeLifecycleGit struct {
	counter fakeCallCounter

	actor         string
	currentBranch string
	currentErr    error
	branchExists  map[string]bool
	resolve       map[string]string
	resolveErr    map[string]error
	diffNames     []string
	diffErr       error
	localBranches []string
	branchesErr   error
	recentCommits []gitx.Commit
	recentErr     error
	status        []gitx.StatusEntry
	statusErr     error
	commonDir     string
	commonDirErr  error
	actionErr     map[string]error
}

func newFakeLifecycleGit(actor, branch string) *fakeLifecycleGit {
	return &fakeLifecycleGit{
		actor:         actor,
		currentBranch: branch,
		branchExists:  map[string]bool{},
		resolve:       map[string]string{},
		resolveErr:    map[string]error{},
		actionErr:     map[string]error{},
	}
}

func (g *fakeLifecycleGit) Calls(name string) int { return g.counter.count(name) }

func (g *fakeLifecycleGit) SeedBranch(branch string, exists bool) {
	if g.branchExists == nil {
		g.branchExists = map[string]bool{}
	}
	g.branchExists[branch] = exists
}

func (g *fakeLifecycleGit) SeedResolve(ref, commit string) {
	if g.resolve == nil {
		g.resolve = map[string]string{}
	}
	g.resolve[ref] = strings.TrimSpace(commit)
}

func (g *fakeLifecycleGit) SeedLocalBranches(branches ...string) {
	g.localBranches = append([]string(nil), branches...)
}

func (g *fakeLifecycleGit) SeedRecentCommits(commits ...gitx.Commit) {
	g.recentCommits = append([]gitx.Commit(nil), commits...)
}

func (g *fakeLifecycleGit) ActionErr(name string) error {
	if g.actionErr == nil {
		return nil
	}
	return g.actionErr[name]
}

func (g *fakeLifecycleGit) CurrentBranch() (string, error) {
	g.counter.hit("CurrentBranch")
	if g.currentErr != nil {
		return "", g.currentErr
	}
	return g.currentBranch, nil
}

func (g *fakeLifecycleGit) BranchExists(branch string) bool {
	g.counter.hit("BranchExists")
	return g.branchExists[branch]
}

func (g *fakeLifecycleGit) DiffNames(baseBranch, branch string) ([]string, error) {
	g.counter.hit("DiffNames")
	if g.diffErr != nil {
		return nil, g.diffErr
	}
	return append([]string(nil), g.diffNames...), nil
}

func (g *fakeLifecycleGit) EnsureBranchExists(branch string) error {
	g.counter.hit("EnsureBranchExists")
	if err := g.ActionErr("EnsureBranchExists"); err != nil {
		return err
	}
	if g.branchExists == nil {
		g.branchExists = map[string]bool{}
	}
	g.branchExists[branch] = true
	return nil
}

func (g *fakeLifecycleGit) EnsureBootstrapCommit(message string) error {
	g.counter.hit("EnsureBootstrapCommit")
	return g.ActionErr("EnsureBootstrapCommit")
}

func (g *fakeLifecycleGit) CreateBranchFrom(branch, ref string) error {
	g.counter.hit("CreateBranchFrom")
	if err := g.ActionErr("CreateBranchFrom"); err != nil {
		return err
	}
	if g.branchExists == nil {
		g.branchExists = map[string]bool{}
	}
	g.branchExists[branch] = true
	g.currentBranch = branch
	if g.resolve == nil {
		g.resolve = map[string]string{}
	}
	if trimmed := strings.TrimSpace(ref); trimmed != "" {
		g.resolve[branch] = trimmed
	}
	return nil
}

func (g *fakeLifecycleGit) CreateBranchAt(branch, ref string) error {
	g.counter.hit("CreateBranchAt")
	if err := g.ActionErr("CreateBranchAt"); err != nil {
		return err
	}
	if g.branchExists == nil {
		g.branchExists = map[string]bool{}
	}
	g.branchExists[branch] = true
	if g.resolve == nil {
		g.resolve = map[string]string{}
	}
	if trimmed := strings.TrimSpace(ref); trimmed != "" {
		g.resolve[branch] = trimmed
	}
	return nil
}

func (g *fakeLifecycleGit) ResolveRef(ref string) (string, error) {
	g.counter.hit("ResolveRef")
	if err, ok := g.resolveErr[ref]; ok && err != nil {
		return "", err
	}
	if value, ok := g.resolve[ref]; ok {
		return value, nil
	}
	return "", fmt.Errorf("resolve ref %q: not found", ref)
}

func (g *fakeLifecycleGit) Actor() string {
	g.counter.hit("Actor")
	return g.actor
}

func (g *fakeLifecycleGit) LocalBranches(prefix string) ([]string, error) {
	g.counter.hit("LocalBranches")
	if g.branchesErr != nil {
		return nil, g.branchesErr
	}
	return append([]string(nil), g.localBranches...), nil
}

func (g *fakeLifecycleGit) RecentCommits(branch string, limit int) ([]gitx.Commit, error) {
	g.counter.hit("RecentCommits")
	if g.recentErr != nil {
		return nil, g.recentErr
	}
	return append([]gitx.Commit(nil), g.recentCommits...), nil
}

func (g *fakeLifecycleGit) RebaseBranchOnto(branch, ontoRef string) error {
	g.counter.hit("RebaseBranchOnto")
	return g.ActionErr("RebaseBranchOnto")
}

func (g *fakeLifecycleGit) RebaseCurrentBranchOnto(ontoRef string) error {
	g.counter.hit("RebaseCurrentBranchOnto")
	return g.ActionErr("RebaseCurrentBranchOnto")
}

func (g *fakeLifecycleGit) WorkingTreeStatus() ([]gitx.StatusEntry, error) {
	g.counter.hit("WorkingTreeStatus")
	if g.statusErr != nil {
		return nil, g.statusErr
	}
	return append([]gitx.StatusEntry(nil), g.status...), nil
}

func (g *fakeLifecycleGit) GitCommonDirAbs() (string, error) {
	g.counter.hit("GitCommonDirAbs")
	if g.commonDirErr != nil {
		return "", g.commonDirErr
	}
	return g.commonDir, nil
}

type fakeStatusGit struct {
	counter fakeCallCounter

	actor         string
	currentBranch string
	currentErr    error
	resolve       map[string]string
	resolveErr    map[string]error
	status        []gitx.StatusEntry
	statusErr     error
}

func newFakeStatusGit(actor, branch string) *fakeStatusGit {
	return &fakeStatusGit{
		actor:         actor,
		currentBranch: branch,
		resolve:       map[string]string{},
		resolveErr:    map[string]error{},
	}
}

func (g *fakeStatusGit) Calls(name string) int { return g.counter.count(name) }

func (g *fakeStatusGit) Actor() string {
	g.counter.hit("Actor")
	return g.actor
}

func (g *fakeStatusGit) CurrentBranch() (string, error) {
	g.counter.hit("CurrentBranch")
	if g.currentErr != nil {
		return "", g.currentErr
	}
	return g.currentBranch, nil
}

func (g *fakeStatusGit) ResolveRef(ref string) (string, error) {
	g.counter.hit("ResolveRef")
	if err, ok := g.resolveErr[ref]; ok && err != nil {
		return "", err
	}
	if value, ok := g.resolve[ref]; ok {
		return value, nil
	}
	return "", fmt.Errorf("resolve ref %q: not found", ref)
}

func (g *fakeStatusGit) WorkingTreeStatus() ([]gitx.StatusEntry, error) {
	g.counter.hit("WorkingTreeStatus")
	if g.statusErr != nil {
		return nil, g.statusErr
	}
	return append([]gitx.StatusEntry(nil), g.status...), nil
}

type fakeMergeGit struct {
	counter fakeCallCounter

	actor         string
	currentBranch string
	currentErr    error
	branchExists  map[string]bool
	resolve       map[string]string
	resolveErr    map[string]error
	status        []gitx.StatusEntry
	statusErr     error
	diffCached    []gitx.FileChange
	diffCachedErr error
	diffNumStat   []gitx.DiffNumStat
	diffNumErr    error
	recentCommits []gitx.Commit
	recentErr     error
	trackedFiles  []string
	trackedErr    error
	worktrees     map[string]*gitx.Worktree
	worktreeErr   map[string]error
	mergeInProg   bool
	mergeErr      error
	mergeFiles    []string
	headShortSHA  string
	headShortErr  error
	mergeHeadSHA  string
	mergeHeadErr  error
	changedCount  int
	changedErr    error
	actionErr     map[string]error
}

func newFakeMergeGit(actor, branch string) *fakeMergeGit {
	return &fakeMergeGit{
		actor:         actor,
		currentBranch: branch,
		branchExists:  map[string]bool{},
		resolve:       map[string]string{},
		resolveErr:    map[string]error{},
		worktrees:     map[string]*gitx.Worktree{},
		worktreeErr:   map[string]error{},
		actionErr:     map[string]error{},
	}
}

func (g *fakeMergeGit) Calls(name string) int { return g.counter.count(name) }
func (g *fakeMergeGit) ActionErr(name string) error {
	if g.actionErr == nil {
		return nil
	}
	return g.actionErr[name]
}

func (g *fakeMergeGit) Actor() string {
	g.counter.hit("Actor")
	return g.actor
}

func (g *fakeMergeGit) BranchExists(branch string) bool {
	g.counter.hit("BranchExists")
	return g.branchExists[branch]
}

func (g *fakeMergeGit) ChangedFileCount(hash string) (int, error) {
	g.counter.hit("ChangedFileCount")
	if g.changedErr != nil {
		return 0, g.changedErr
	}
	return g.changedCount, nil
}

func (g *fakeMergeGit) CheckoutBranch(branch string) error {
	g.counter.hit("CheckoutBranch")
	if err := g.ActionErr("CheckoutBranch"); err != nil {
		return err
	}
	g.currentBranch = branch
	return nil
}

func (g *fakeMergeGit) Commit(message string) error {
	g.counter.hit("Commit")
	return g.ActionErr("Commit")
}

func (g *fakeMergeGit) CurrentBranch() (string, error) {
	g.counter.hit("CurrentBranch")
	if g.currentErr != nil {
		return "", g.currentErr
	}
	return g.currentBranch, nil
}

func (g *fakeMergeGit) DeleteBranch(branch string, force bool) error {
	g.counter.hit("DeleteBranch")
	if err := g.ActionErr("DeleteBranch"); err != nil {
		return err
	}
	delete(g.branchExists, branch)
	return nil
}

func (g *fakeMergeGit) DiffNameStatusCached() ([]gitx.FileChange, error) {
	g.counter.hit("DiffNameStatusCached")
	if g.diffCachedErr != nil {
		return nil, g.diffCachedErr
	}
	return append([]gitx.FileChange(nil), g.diffCached...), nil
}

func (g *fakeMergeGit) DiffNumStatCached() ([]gitx.DiffNumStat, error) {
	g.counter.hit("DiffNumStatCached")
	if g.diffNumErr != nil {
		return nil, g.diffNumErr
	}
	return append([]gitx.DiffNumStat(nil), g.diffNumStat...), nil
}

func (g *fakeMergeGit) HeadShortSHA() (string, error) {
	g.counter.hit("HeadShortSHA")
	if g.headShortErr != nil {
		return "", g.headShortErr
	}
	return g.headShortSHA, nil
}

func (g *fakeMergeGit) IsMergeInProgress() (bool, error) {
	g.counter.hit("IsMergeInProgress")
	if g.mergeErr != nil {
		return false, g.mergeErr
	}
	return g.mergeInProg, nil
}

func (g *fakeMergeGit) MergeAbort() error {
	g.counter.hit("MergeAbort")
	if err := g.ActionErr("MergeAbort"); err != nil {
		return err
	}
	g.mergeInProg = false
	return nil
}

func (g *fakeMergeGit) MergeConflictFiles() ([]string, error) {
	g.counter.hit("MergeConflictFiles")
	if g.mergeErr != nil {
		return nil, g.mergeErr
	}
	return append([]string(nil), g.mergeFiles...), nil
}

func (g *fakeMergeGit) MergeContinue() error {
	g.counter.hit("MergeContinue")
	if err := g.ActionErr("MergeContinue"); err != nil {
		return err
	}
	g.mergeInProg = false
	return nil
}

func (g *fakeMergeGit) MergeHeadSHA() (string, error) {
	g.counter.hit("MergeHeadSHA")
	if g.mergeHeadErr != nil {
		return "", g.mergeHeadErr
	}
	return g.mergeHeadSHA, nil
}

func (g *fakeMergeGit) MergeNoFFNoCommitOnCurrentBranch(branch, message string) error {
	g.counter.hit("MergeNoFFNoCommitOnCurrentBranch")
	if err := g.ActionErr("MergeNoFFNoCommitOnCurrentBranch"); err != nil {
		return err
	}
	g.mergeInProg = true
	return nil
}

func (g *fakeMergeGit) RecentCommits(branch string, limit int) ([]gitx.Commit, error) {
	g.counter.hit("RecentCommits")
	if g.recentErr != nil {
		return nil, g.recentErr
	}
	return append([]gitx.Commit(nil), g.recentCommits...), nil
}

func (g *fakeMergeGit) RebaseBranchOnto(branch, ontoRef string) error {
	g.counter.hit("RebaseBranchOnto")
	return g.ActionErr("RebaseBranchOnto")
}

func (g *fakeMergeGit) RebaseCurrentBranchOnto(ontoRef string) error {
	g.counter.hit("RebaseCurrentBranchOnto")
	return g.ActionErr("RebaseCurrentBranchOnto")
}

func (g *fakeMergeGit) ResolveRef(ref string) (string, error) {
	g.counter.hit("ResolveRef")
	if err, ok := g.resolveErr[ref]; ok && err != nil {
		return "", err
	}
	if value, ok := g.resolve[ref]; ok {
		return value, nil
	}
	return "", fmt.Errorf("resolve ref %q: not found", ref)
}

func (g *fakeMergeGit) StagePaths(paths []string) error {
	g.counter.hit("StagePaths")
	return g.ActionErr("StagePaths")
}

func (g *fakeMergeGit) TrackedFiles(pathspec string) ([]string, error) {
	g.counter.hit("TrackedFiles")
	if g.trackedErr != nil {
		return nil, g.trackedErr
	}
	return append([]string(nil), g.trackedFiles...), nil
}

func (g *fakeMergeGit) WorkingTreeStatus() ([]gitx.StatusEntry, error) {
	g.counter.hit("WorkingTreeStatus")
	if g.statusErr != nil {
		return nil, g.statusErr
	}
	return append([]gitx.StatusEntry(nil), g.status...), nil
}

func (g *fakeMergeGit) WorktreeForBranch(branch string) (*gitx.Worktree, error) {
	g.counter.hit("WorktreeForBranch")
	if err, ok := g.worktreeErr[branch]; ok && err != nil {
		return nil, err
	}
	if wt, ok := g.worktrees[branch]; ok && wt != nil {
		copyWT := *wt
		return &copyWT, nil
	}
	return nil, nil
}

func (g *fakeMergeGit) clone() *fakeMergeGit {
	out := &fakeMergeGit{
		actor:         g.actor,
		currentBranch: g.currentBranch,
		currentErr:    g.currentErr,
		statusErr:     g.statusErr,
		diffCachedErr: g.diffCachedErr,
		diffNumErr:    g.diffNumErr,
		recentErr:     g.recentErr,
		trackedErr:    g.trackedErr,
		mergeInProg:   g.mergeInProg,
		mergeErr:      g.mergeErr,
		mergeFiles:    append([]string(nil), g.mergeFiles...),
		headShortSHA:  g.headShortSHA,
		headShortErr:  g.headShortErr,
		mergeHeadSHA:  g.mergeHeadSHA,
		mergeHeadErr:  g.mergeHeadErr,
		changedCount:  g.changedCount,
		changedErr:    g.changedErr,
		branchExists:  cloneBoolMapHarness(g.branchExists),
		resolve:       cloneStringMapHarness(g.resolve),
		resolveErr:    cloneErrorMapHarness(g.resolveErr),
		status:        append([]gitx.StatusEntry(nil), g.status...),
		diffCached:    append([]gitx.FileChange(nil), g.diffCached...),
		diffNumStat:   append([]gitx.DiffNumStat(nil), g.diffNumStat...),
		recentCommits: append([]gitx.Commit(nil), g.recentCommits...),
		trackedFiles:  append([]string(nil), g.trackedFiles...),
		worktrees:     map[string]*gitx.Worktree{},
		worktreeErr:   cloneErrorMapHarness(g.worktreeErr),
		actionErr:     cloneErrorMapHarness(g.actionErr),
	}
	for key, value := range g.worktrees {
		if value == nil {
			out.worktrees[key] = nil
			continue
		}
		copyWT := *value
		out.worktrees[key] = &copyWT
	}
	return out
}

type fakeTaskGit struct {
	counter fakeCallCounter

	actor         string
	currentBranch string
	currentErr    error
	hasStaged     bool
	hasStagedErr  error
	diff          string
	diffErr       error
	status        []gitx.StatusEntry
	statusErr     error
	headShortSHA  string
	headShortErr  error
	pathChanges   map[string]bool
	pathChangeErr error
	actionErr     map[string]error
	commitMsgs    []string
}

func newFakeTaskGit(actor, branch string) *fakeTaskGit {
	return &fakeTaskGit{
		actor:         actor,
		currentBranch: branch,
		pathChanges:   map[string]bool{},
		actionErr:     map[string]error{},
	}
}

func (g *fakeTaskGit) Calls(name string) int { return g.counter.count(name) }
func (g *fakeTaskGit) ActionErr(name string) error {
	if g.actionErr == nil {
		return nil
	}
	return g.actionErr[name]
}

func (g *fakeTaskGit) Actor() string {
	g.counter.hit("Actor")
	return g.actor
}

func (g *fakeTaskGit) CurrentBranch() (string, error) {
	g.counter.hit("CurrentBranch")
	if g.currentErr != nil {
		return "", g.currentErr
	}
	return g.currentBranch, nil
}

func (g *fakeTaskGit) HasStagedChanges() (bool, error) {
	g.counter.hit("HasStagedChanges")
	if g.hasStagedErr != nil {
		return false, g.hasStagedErr
	}
	return g.hasStaged, nil
}

func (g *fakeTaskGit) WorkingTreeUnifiedDiff(paths []string, contextLines int) (string, error) {
	g.counter.hit("WorkingTreeUnifiedDiff")
	if g.diffErr != nil {
		return "", g.diffErr
	}
	return g.diff, nil
}

func (g *fakeTaskGit) StageAll() error {
	g.counter.hit("StageAll")
	return g.ActionErr("StageAll")
}

func (g *fakeTaskGit) StagePaths(paths []string) error {
	g.counter.hit("StagePaths")
	return g.ActionErr("StagePaths")
}

func (g *fakeTaskGit) ApplyPatchToIndex(patchPath string) error {
	g.counter.hit("ApplyPatchToIndex")
	return g.ActionErr("ApplyPatchToIndex")
}

func (g *fakeTaskGit) PathHasChanges(path string) (bool, error) {
	g.counter.hit("PathHasChanges")
	if g.pathChangeErr != nil {
		return false, g.pathChangeErr
	}
	return g.pathChanges[path], nil
}

func (g *fakeTaskGit) WorkingTreeStatus() ([]gitx.StatusEntry, error) {
	g.counter.hit("WorkingTreeStatus")
	if g.statusErr != nil {
		return nil, g.statusErr
	}
	return append([]gitx.StatusEntry(nil), g.status...), nil
}

func (g *fakeTaskGit) Commit(msg string) error {
	g.counter.hit("Commit")
	g.commitMsgs = append(g.commitMsgs, msg)
	return g.ActionErr("Commit")
}

func (g *fakeTaskGit) HeadShortSHA() (string, error) {
	g.counter.hit("HeadShortSHA")
	if g.headShortErr != nil {
		return "", g.headShortErr
	}
	return g.headShortSHA, nil
}

func cloneHarnessCR(cr *model.CR) *model.CR {
	if cr == nil {
		return nil
	}
	out := *cr
	out.Notes = append([]string(nil), cr.Notes...)
	out.Evidence = append([]model.EvidenceEntry(nil), cr.Evidence...)
	out.Contract = cloneContract(cr.Contract)
	out.Subtasks = cloneSubtasks(cr.Subtasks)
	out.Events = append([]model.Event(nil), cr.Events...)
	return &out
}

func cloneBoolMapHarness(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringMapHarness(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneErrorMapHarness(in map[string]error) map[string]error {
	out := map[string]error{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

var (
	_ lifecycleRuntimeStore      = (*fakeCRStore)(nil)
	_ statusRuntimeStore         = (*fakeCRStore)(nil)
	_ mergeRuntimeStore          = (*fakeCRStore)(nil)
	_ taskLifecycleStoreProvider = (*fakeCRStore)(nil)

	_ lifecycleRuntimeGit      = (*fakeLifecycleGit)(nil)
	_ statusRuntimeGit         = (*fakeStatusGit)(nil)
	_ mergeRuntimeGit          = (*fakeMergeGit)(nil)
	_ taskLifecycleGitProvider = (*fakeTaskGit)(nil)
)
