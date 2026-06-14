package plane

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// Resolver provides thread-safe name-to-UUID and UUID-to-name resolution.
type Resolver struct {
	client *Client

	// Cache maps
	projectsByID         map[string]*Project
	projectsByName       map[string]*Project
	projectsByIdentifier map[string]*Project

	// projectID -> stateID -> State
	statesByID map[string]map[string]*State
	// projectID -> lowercase stateName/slug -> State
	statesByName map[string]map[string]*State

	// projectID -> labelID -> Label
	labelsByID map[string]map[string]*Label
	// projectID -> lowercase labelName -> Label
	labelsByName map[string]map[string]*Label

	// projectID -> moduleID -> Module
	modulesByID map[string]map[string]*Module
	// projectID -> lowercase moduleName -> Module
	modulesByName map[string]map[string]*Module

	membersByID    map[string]*Member
	membersByName  map[string]*Member
	membersByEmail map[string]*Member

	callerID string // cached once; empty means not yet fetched
	callerMu sync.Mutex

	mu sync.RWMutex
}

// NewResolver creates a new resolver instance
func NewResolver(client *Client) *Resolver {
	return &Resolver{
		client:               client,
		projectsByID:         make(map[string]*Project),
		projectsByName:       make(map[string]*Project),
		projectsByIdentifier: make(map[string]*Project),
		statesByID:           make(map[string]map[string]*State),
		statesByName:         make(map[string]map[string]*State),
		labelsByID:           make(map[string]map[string]*Label),
		labelsByName:         make(map[string]map[string]*Label),
		modulesByID:          make(map[string]map[string]*Module),
		modulesByName:        make(map[string]map[string]*Module),
		membersByID:          make(map[string]*Member),
		membersByName:        make(map[string]*Member),
		membersByEmail:       make(map[string]*Member),
	}
}

// fetchProjects loads all projects from the API and updates the cache.
func (r *Resolver) fetchProjects(ctx context.Context) error {
	projects, err := r.client.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch projects for cache: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing projects cache
	r.projectsByID = make(map[string]*Project)
	r.projectsByName = make(map[string]*Project)
	r.projectsByIdentifier = make(map[string]*Project)

	for i := range projects {
		p := &projects[i]
		r.projectsByID[p.ID] = p
		r.projectsByName[strings.ToLower(p.Name)] = p
		r.projectsByIdentifier[strings.ToLower(p.Identifier)] = p
	}

	return nil
}

// fetchStates loads all states for a project and updates the cache.
func (r *Resolver) fetchStates(ctx context.Context, projectID string) error {
	states, err := r.client.ListStates(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to fetch states for cache: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.statesByID[projectID]; !ok {
		r.statesByID[projectID] = make(map[string]*State)
		r.statesByName[projectID] = make(map[string]*State)
	}

	// Clear project-specific states cache
	for k := range r.statesByID[projectID] {
		delete(r.statesByID[projectID], k)
	}
	for k := range r.statesByName[projectID] {
		delete(r.statesByName[projectID], k)
	}

	for i := range states {
		s := &states[i]
		r.statesByID[projectID][s.ID] = s
		r.statesByName[projectID][strings.ToLower(s.Name)] = s
	}

	return nil
}

// fetchLabels loads all labels for a project and updates the cache.
func (r *Resolver) fetchLabels(ctx context.Context, projectID string) error {
	labels, err := r.client.ListLabels(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to fetch labels for cache: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.labelsByID[projectID]; !ok {
		r.labelsByID[projectID] = make(map[string]*Label)
		r.labelsByName[projectID] = make(map[string]*Label)
	}

	// Clear project-specific labels cache
	for k := range r.labelsByID[projectID] {
		delete(r.labelsByID[projectID], k)
	}
	for k := range r.labelsByName[projectID] {
		delete(r.labelsByName[projectID], k)
	}

	for i := range labels {
		l := &labels[i]
		r.labelsByID[projectID][l.ID] = l
		r.labelsByName[projectID][strings.ToLower(l.Name)] = l
	}

	return nil
}

// fetchModules loads all modules for a project and updates the cache.
func (r *Resolver) fetchModules(ctx context.Context, projectID string) error {
	modules, err := r.client.ListModules(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to fetch modules for cache: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.modulesByID[projectID]; !ok {
		r.modulesByID[projectID] = make(map[string]*Module)
		r.modulesByName[projectID] = make(map[string]*Module)
	}

	// Clear project-specific modules cache
	for k := range r.modulesByID[projectID] {
		delete(r.modulesByID[projectID], k)
	}
	for k := range r.modulesByName[projectID] {
		delete(r.modulesByName[projectID], k)
	}

	for i := range modules {
		m := &modules[i]
		r.modulesByID[projectID][m.ID] = m
		r.modulesByName[projectID][strings.ToLower(m.Name)] = m
	}

	return nil
}

// fetchMembers loads all workspace members and updates the cache.
func (r *Resolver) fetchMembers(ctx context.Context) error {
	members, err := r.client.ListWorkspaceMembers(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch members for cache: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear members cache
	r.membersByID = make(map[string]*Member)
	r.membersByName = make(map[string]*Member)
	r.membersByEmail = make(map[string]*Member)

	for i := range members {
		m := &members[i]
		r.membersByID[m.ID] = m
		r.membersByEmail[strings.ToLower(m.Email)] = m
		if m.DisplayName != "" {
			r.membersByName[strings.ToLower(m.DisplayName)] = m
		} else {
			fullName := strings.ToLower(fmt.Sprintf("%s %s", m.FirstName, m.LastName))
			r.membersByName[fullName] = m
		}
	}

	return nil
}

// GetCallerID returns the UUID of the workspace member whose API key is in use.
// Result is cached after the first call.
func (r *Resolver) GetCallerID(ctx context.Context) (string, error) {
	r.callerMu.Lock()
	defer r.callerMu.Unlock()
	if r.callerID != "" {
		return r.callerID, nil
	}
	me, err := r.client.GetMe(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to identify caller: %w", err)
	}
	r.callerID = me.ID
	return r.callerID, nil
}

// ResolveProject resolves a project by name, identifier prefix, or UUID.
func (r *Resolver) ResolveProject(ctx context.Context, input string) (*Project, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("project input cannot be empty")
	}

	// Helper to lookup inside locked read context
	lookup := func() *Project {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if isUUID(input) {
			return r.projectsByID[input]
		}
		lowerInput := strings.ToLower(input)
		if p, ok := r.projectsByIdentifier[lowerInput]; ok {
			return p
		}
		if p, ok := r.projectsByName[lowerInput]; ok {
			return p
		}
		return nil
	}

	// 1. Try cache lookup
	if p := lookup(); p != nil {
		return p, nil
	}

	// 2. Fetch fresh projects and retry
	if err := r.fetchProjects(ctx); err != nil {
		return nil, err
	}

	if p := lookup(); p != nil {
		return p, nil
	}

	return nil, fmt.Errorf("project not found: %s", input)
}

// ResolveState resolves a state by name or UUID inside a specific project.
func (r *Resolver) ResolveState(ctx context.Context, projectID string, input string) (*State, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("state input cannot be empty")
	}

	lookup := func() *State {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if isUUID(input) {
			if projectStates, ok := r.statesByID[projectID]; ok {
				return projectStates[input]
			}
			return nil
		}

		lowerInput := strings.ToLower(input)
		if projectStates, ok := r.statesByName[projectID]; ok {
			return projectStates[lowerInput]
		}
		return nil
	}

	// 1. Try cache lookup
	if s := lookup(); s != nil {
		return s, nil
	}

	// 2. Fetch fresh states and retry
	if err := r.fetchStates(ctx, projectID); err != nil {
		return nil, err
	}

	if s := lookup(); s != nil {
		return s, nil
	}

	return nil, fmt.Errorf("state not found for project %s: %s", projectID, input)
}

// ResolveLabel resolves a label by name or UUID inside a specific project.
func (r *Resolver) ResolveLabel(ctx context.Context, projectID string, input string) (*Label, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("label input cannot be empty")
	}

	lookup := func() *Label {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if isUUID(input) {
			if projectLabels, ok := r.labelsByID[projectID]; ok {
				return projectLabels[input]
			}
			return nil
		}

		lowerInput := strings.ToLower(input)
		if projectLabels, ok := r.labelsByName[projectID]; ok {
			return projectLabels[lowerInput]
		}
		return nil
	}

	// 1. Try cache lookup
	if l := lookup(); l != nil {
		return l, nil
	}

	// 2. Fetch fresh labels and retry
	if err := r.fetchLabels(ctx, projectID); err != nil {
		return nil, err
	}

	if l := lookup(); l != nil {
		return l, nil
	}

	return nil, fmt.Errorf("label not found for project %s: %s", projectID, input)
}

// ResolveModule resolves a module by name or UUID inside a specific project.
func (r *Resolver) ResolveModule(ctx context.Context, projectID string, input string) (*Module, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("module input cannot be empty")
	}

	lookup := func() *Module {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if isUUID(input) {
			if projectModules, ok := r.modulesByID[projectID]; ok {
				return projectModules[input]
			}
			return nil
		}

		lowerInput := strings.ToLower(input)
		if projectModules, ok := r.modulesByName[projectID]; ok {
			return projectModules[lowerInput]
		}
		return nil
	}

	// 1. Try cache lookup
	if m := lookup(); m != nil {
		return m, nil
	}

	// 2. Fetch fresh modules and retry
	if err := r.fetchModules(ctx, projectID); err != nil {
		return nil, err
	}

	if m := lookup(); m != nil {
		return m, nil
	}

	return nil, fmt.Errorf("module not found for project %s: %s", projectID, input)
}

// ResolveMember resolves a workspace member by name, display name, email, or UUID.
func (r *Resolver) ResolveMember(ctx context.Context, input string) (*Member, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("member input cannot be empty")
	}

	lookup := func() *Member {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if isUUID(input) {
			return r.membersByID[input]
		}

		lowerInput := strings.ToLower(input)
		if m, ok := r.membersByEmail[lowerInput]; ok {
			return m
		}
		if m, ok := r.membersByName[lowerInput]; ok {
			return m
		}
		return nil
	}

	// 1. Try cache lookup
	if m := lookup(); m != nil {
		return m, nil
	}

	// 2. Fetch fresh members and retry
	if err := r.fetchMembers(ctx); err != nil {
		return nil, err
	}

	if m := lookup(); m != nil {
		return m, nil
	}

	return nil, fmt.Errorf("member not found: %s", input)
}

// ResolvedWorkItem represents a WorkItem but with fully resolved names instead of UUID strings
type ResolvedWorkItem struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	DescriptionHTML     string    `json:"description_html,omitempty"`
	DescriptionStripped string    `json:"description_stripped,omitempty"`
	Priority            string    `json:"priority,omitempty"`
	StartDate           string    `json:"start_date,omitempty"`
	TargetDate          string    `json:"target_date,omitempty"`
	SequenceID          int       `json:"sequence_id"`
	SortOrder           float64   `json:"sort_order"`
	CompletedAt         string    `json:"completed_at,omitempty"`
	ArchivedAt          string    `json:"archived_at,omitempty"`
	IsDraft             bool      `json:"is_draft"`
	ProjectName         string    `json:"project_name"`
	ProjectIdentifier   string    `json:"project_identifier"`
	ProjectID           string    `json:"project_id"`
	StateName           string    `json:"state_name"`
	StateGroup          string    `json:"state_group"`
	StateID             string    `json:"state_id"`
	AssigneeNames       []string  `json:"assignee_names"`
	AssigneeEmails      []string  `json:"assignee_emails"`
	LabelNames          []string  `json:"label_names"`
}

// ResolveWorkItem maps all UUID references inside a WorkItem to their human-readable name counterparts.
func (r *Resolver) ResolveWorkItem(ctx context.Context, item *WorkItem) (*ResolvedWorkItem, error) {
	resolved := &ResolvedWorkItem{
		ID:                  item.ID,
		Name:                item.Name,
		DescriptionHTML:     item.DescriptionHTML,
		DescriptionStripped: item.DescriptionStripped,
		Priority:            item.Priority,
		StartDate:           item.StartDate,
		TargetDate:          item.TargetDate,
		SequenceID:          item.SequenceID,
		SortOrder:           item.SortOrder,
		CompletedAt:         item.CompletedAt,
		ArchivedAt:          item.ArchivedAt,
		IsDraft:             item.IsDraft,
	}

	// 1. Resolve Project
	projectInput := item.Project.ID
	if item.Project.Val != nil {
		resolved.ProjectID = item.Project.Val.ID
		resolved.ProjectName = item.Project.Val.Name
		resolved.ProjectIdentifier = item.Project.Val.Identifier
	} else if projectInput != "" {
		proj, err := r.ResolveProject(ctx, projectInput)
		if err == nil {
			resolved.ProjectID = proj.ID
			resolved.ProjectName = proj.Name
			resolved.ProjectIdentifier = proj.Identifier
		} else {
			resolved.ProjectID = projectInput
		}
	}

	// 2. Resolve State
	stateInput := item.State.ID
	if item.State.Val != nil {
		resolved.StateID = item.State.Val.ID
		resolved.StateName = item.State.Val.Name
		resolved.StateGroup = item.State.Val.Group
	} else if stateInput != "" && resolved.ProjectID != "" {
		st, err := r.ResolveState(ctx, resolved.ProjectID, stateInput)
		if err == nil {
			resolved.StateID = st.ID
			resolved.StateName = st.Name
			resolved.StateGroup = st.Group
		} else {
			resolved.StateID = stateInput
		}
	}

	// 3. Resolve Assignees
	resolved.AssigneeNames = []string{}
	resolved.AssigneeEmails = []string{}
	for _, a := range item.Assignees {
		if a.Val != nil {
			resolved.AssigneeEmails = append(resolved.AssigneeEmails, a.Val.Email)
			if a.Val.DisplayName != "" {
				resolved.AssigneeNames = append(resolved.AssigneeNames, a.Val.DisplayName)
			} else {
				resolved.AssigneeNames = append(resolved.AssigneeNames, fmt.Sprintf("%s %s", a.Val.FirstName, a.Val.LastName))
			}
		} else if a.ID != "" {
			mem, err := r.ResolveMember(ctx, a.ID)
			if err == nil {
				resolved.AssigneeEmails = append(resolved.AssigneeEmails, mem.Email)
				if mem.DisplayName != "" {
					resolved.AssigneeNames = append(resolved.AssigneeNames, mem.DisplayName)
				} else {
					resolved.AssigneeNames = append(resolved.AssigneeNames, fmt.Sprintf("%s %s", mem.FirstName, mem.LastName))
				}
			} else {
				resolved.AssigneeNames = append(resolved.AssigneeNames, a.ID)
			}
		}
	}

	// 4. Resolve Labels
	resolved.LabelNames = []string{}
	for _, l := range item.Labels {
		if l.Val != nil {
			resolved.LabelNames = append(resolved.LabelNames, l.Val.Name)
		} else if l.ID != "" && resolved.ProjectID != "" {
			lbl, err := r.ResolveLabel(ctx, resolved.ProjectID, l.ID)
			if err == nil {
				resolved.LabelNames = append(resolved.LabelNames, lbl.Name)
			} else {
				resolved.LabelNames = append(resolved.LabelNames, l.ID)
			}
		}
	}

	return resolved, nil
}
