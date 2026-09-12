package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shreemangalam/stratum/server/internal/cache"
	"github.com/shreemangalam/stratum/server/internal/core"
	"github.com/shreemangalam/stratum/server/internal/http/generated"
	"github.com/shreemangalam/stratum/server/internal/store"
)

type (
	createDiffRequest = generated.CreateDiffRequest
	errorResponse     = generated.ErrorResponse
	languageInfo      = generated.LanguageInfo
	languagesResponse = generated.LanguagesResponse
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	resp := map[string]string{"status": "ok"}
	if err := s.store.Ping(ctx); err != nil {
		resp["status"] = "degraded"
		resp["database"] = "unreachable"
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	resp["database"] = "connected"
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleLanguages(w http.ResponseWriter, _ *http.Request) {
	langs := s.registry.Languages()
	infos := make([]languageInfo, 0, len(langs))
	for _, lang := range langs {
		p, err := s.registry.ForLanguage(lang)
		if err != nil {
			continue
		}
		infos = append(infos, languageInfo{
			Id:         p.Language(),
			Extensions: p.Extensions(),
		})
	}
	writeJSON(w, http.StatusOK, languagesResponse{Languages: infos})
}

func (s *Server) handleCreateDiff(w http.ResponseWriter, r *http.Request) {
	var req createDiffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err.Error() == "http: request body too large" {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.Left.Content == "" || req.Right.Content == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "both left and right content are required"})
		return
	}

	lang := optionalString(req.Language)
	if lang == "" {
		lang = detectLanguage(optionalString(req.Left.Filename), optionalString(req.Right.Filename), s)
	}
	if lang == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "could not detect language from filename - please select a language",
		})
		return
	}

	if _, err := s.registry.ForLanguage(lang); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("unsupported language: %s", lang),
		})
		return
	}

	leftBytes := []byte(req.Left.Content)
	rightBytes := []byte(req.Right.Content)
	leftHash := cache.ContentHash(leftBytes)
	rightHash := cache.ContentHash(rightBytes)

	existing, err := s.store.FindJobByHashes(r.Context(), leftHash, rightHash, lang)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		return
	}
	if existing != nil {
		if existing.Status == store.StatusFailed {
			s.sources.Put(leftHash, leftBytes)
			s.sources.Put(rightHash, rightBytes)
			requeued, reqErr := s.store.RequeueFailedJob(r.Context(), existing.ID, req.Left.Content, req.Right.Content)
			if reqErr != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
				return
			}
			if requeued != nil {
				writeJSON(w, http.StatusOK, requeued)
				return
			}
		}
		writeJSON(w, http.StatusOK, existing)
		return
	}

	s.sources.Put(leftHash, leftBytes)
	s.sources.Put(rightHash, rightBytes)

	job, err := s.store.CreateJob(r.Context(), leftHash, rightHash, lang, req.Left.Content, req.Right.Content)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create job"})
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func (s *Server) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "missing diff id"})
		return
	}

	if !isValidUUID(id) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "diff not found"})
		return
	}

	job, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		return
	}
	if job == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "diff not found"})
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleStreamDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !isValidUUID(id) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "diff not found"})
		return
	}

	job, err := s.store.GetJob(r.Context(), id)
	if err != nil || job == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "diff not found"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if job.Status == store.StatusCompleted || job.Status == store.StatusFailed {
		if err := writeSSE(w, "status", job.Status); err != nil {
			return
		}
		if job.Status == store.StatusCompleted && job.Result != nil {
			if err := writeSSE(w, "result", string(job.Result)); err != nil {
				return
			}
		}
		if job.Status == store.StatusFailed && job.Error != "" {
			if err := writeSSE(w, "error", job.Error); err != nil {
				return
			}
		}
		flusher.Flush()
		return
	}

	ch := s.subscribers.Subscribe(id)
	defer s.subscribers.Unsubscribe(id, ch)

	job, err = s.store.GetJob(r.Context(), id)
	if err != nil || job == nil {
		return
	}
	if job.Status == store.StatusCompleted || job.Status == store.StatusFailed {
		if err := writeSSE(w, "status", job.Status); err != nil {
			return
		}
		if job.Status == store.StatusCompleted && job.Result != nil {
			if err := writeSSE(w, "result", string(job.Result)); err != nil {
				return
			}
		}
		if job.Status == store.StatusFailed && job.Error != "" {
			if err := writeSSE(w, "error", job.Error); err != nil {
				return
			}
		}
		flusher.Flush()
		return
	}

	if err := writeSSE(w, "status", job.Status); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, event.Type, event.Data); err != nil {
				return
			}
			flusher.Flush()

			if event.Type == "result" || (event.Type == "status" && (event.Data == "completed" || event.Data == "failed")) {
				return
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, event string, data any) error {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

type statsResponse struct {
	Jobs        *store.JobStats `json:"jobs"`
	CacheSize   int             `json:"cache_size"`
	Subscribers int             `json:"active_subscribers"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	stats, err := s.store.JobStats(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to retrieve stats"})
		return
	}

	writeJSON(w, http.StatusOK, statsResponse{
		Jobs:        stats,
		CacheSize:   s.cache.Size(),
		Subscribers: s.subscribers.Count(),
	})
}

type (
	createMergeRequest = generated.CreateMergeRequest
	mergeResponse      = generated.MergeResponse
	mergeEntryGen      = generated.MergeEntry
	mergePlanGen       = generated.MergePlan
)

func (s *Server) handleCreateMerge(w http.ResponseWriter, r *http.Request) {
	var req createMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err.Error() == "http: request body too large" {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.Base.Content == "" || req.Left.Content == "" || req.Right.Content == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "base, left, and right content are required"})
		return
	}

	lang := optionalString(req.Language)
	if lang == "" {
		lang = detectLanguage(optionalString(req.Base.Filename), optionalString(req.Left.Filename), s)
	}
	if lang == "" {
		lang = detectLanguage(optionalString(req.Right.Filename), "", s)
	}
	if lang == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "could not detect language from filename - please select a language",
		})
		return
	}

	parser, err := s.registry.ForLanguage(lang)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("unsupported language: %s", lang),
		})
		return
	}

	baseTree, err := parser.Parse(r.Context(), []byte(req.Base.Content))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("failed to parse base: %v", err),
		})
		return
	}
	leftTree, err := parser.Parse(r.Context(), []byte(req.Left.Content))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("failed to parse left: %v", err),
		})
		return
	}
	rightTree, err := parser.Parse(r.Context(), []byte(req.Right.Content))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("failed to parse right: %v", err),
		})
		return
	}

	plan, err := core.PlanThreeWayMerge(baseTree, leftTree, rightTree, core.DefaultMatchConfig())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("merge planning failed: %v", err),
		})
		return
	}

	entries := make([]mergeEntryGen, len(plan.Entries))
	for i, e := range plan.Entries {
		entries[i] = mergeEntryGen{
			BaseNode:  toNodeRefPtr(e.BaseNode),
			LeftNode:  toNodeRefPtr(e.LeftNode),
			RightNode: toNodeRefPtr(e.RightNode),
			Decision:  generated.MergeEntryDecision(e.Decision),
			Reason:    e.Reason,
		}
		if e.ConflictKind != nil {
			kind := generated.MergeEntryConflictKind(*e.ConflictKind)
			entries[i].ConflictKind = &kind
		}
	}

	mergedSource := core.GenerateMergedSource(plan, baseTree, leftTree, rightTree)

	resp := mergeResponse{
		Language: lang,
		Plan: mergePlanGen{
			Entries:       entries,
			ConflictCount: plan.ConflictCount,
			HasConflicts:  plan.HasConflicts,
		},
		BaseSource:   &req.Base.Content,
		LeftSource:   &req.Left.Content,
		RightSource:  &req.Right.Content,
		MergedSource: &mergedSource,
	}

	writeJSON(w, http.StatusOK, resp)
}

func toNodeRefPtr(ref *core.NodeRef) *generated.NodeRef {
	if ref == nil {
		return nil
	}
	return &generated.NodeRef{
		Id:   int(ref.ID),
		Path: ref.Path,
		Kind: ref.Kind,
		Label: func() *string {
			if ref.Label == "" {
				return nil
			}
			return &ref.Label
		}(),
		Location: generated.Location{
			Line:   ref.Location.Line,
			Column: ref.Location.Column,
			Offset: ref.Location.Offset,
		},
	}
}

type gitDiffRequest = generated.GitDiffRequest

func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	var req gitDiffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.RepoPath == "" || req.FilePath == "" || req.LeftRef == "" || req.RightRef == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "repo_path, file_path, left_ref, and right_ref are required",
		})
		return
	}

	if err := validateRepoPath(req.RepoPath); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if !isCleanRef(req.LeftRef) || !isCleanRef(req.RightRef) || !isCleanPath(req.FilePath) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid ref or file path"})
		return
	}

	leftContent, err := gitShow(r.Context(), req.RepoPath, req.LeftRef, req.FilePath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("failed to read %s at %s: %v", req.FilePath, req.LeftRef, err),
		})
		return
	}

	rightContent, err := gitShow(r.Context(), req.RepoPath, req.RightRef, req.FilePath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("failed to read %s at %s: %v", req.FilePath, req.RightRef, err),
		})
		return
	}

	lang := optionalString(req.Language)
	if lang == "" {
		ext := strings.TrimPrefix(filepath.Ext(req.FilePath), ".")
		if p, err := s.registry.ForExtension(ext); err == nil {
			lang = p.Language()
		}
	}
	if lang == "" {
		lang = "c" // line-based fallback for unrecognized extensions
	}

	leftBytes := []byte(leftContent)
	rightBytes := []byte(rightContent)
	leftHash := cache.ContentHash(leftBytes)
	rightHash := cache.ContentHash(rightBytes)

	existing, err := s.store.FindJobByHashes(r.Context(), leftHash, rightHash, lang)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		return
	}
	if existing != nil {
		if existing.Status == store.StatusFailed {
			s.sources.Put(leftHash, leftBytes)
			s.sources.Put(rightHash, rightBytes)
			requeued, reqErr := s.store.RequeueFailedJob(r.Context(), existing.ID, leftContent, rightContent)
			if reqErr != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
				return
			}
			if requeued != nil {
				writeJSON(w, http.StatusOK, requeued)
				return
			}
		}
		writeJSON(w, http.StatusOK, existing)
		return
	}

	s.sources.Put(leftHash, leftBytes)
	s.sources.Put(rightHash, rightBytes)

	job, err := s.store.CreateJob(r.Context(), leftHash, rightHash, lang, leftContent, rightContent)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create job"})
		return
	}

	writeJSON(w, http.StatusCreated, job)
}

func gitShow(ctx context.Context, repoPath, ref, filePath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "show", ref+":"+filePath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isCleanRef(ref string) bool {
	for _, r := range ref {
		if r == ';' || r == '|' || r == '&' || r == '$' || r == '`' || r == '\n' || r == '\r' {
			return false
		}
	}
	return len(ref) > 0 && len(ref) <= 256
}

func isCleanPath(p string) bool {
	if strings.Contains(p, "..") || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return false
	}
	for _, r := range p {
		if r == ';' || r == '|' || r == '&' || r == '$' || r == '`' || r == '\n' || r == '\r' {
			return false
		}
	}
	return len(p) > 0 && len(p) <= 1024
}

func validateRepoPath(repoPath string) error {
	if !filepath.IsAbs(repoPath) {
		return fmt.Errorf("repo_path must be an absolute path")
	}
	cleaned := filepath.Clean(repoPath)
	gitDir := filepath.Join(cleaned, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("repo_path does not appear to be a git repository (no .git directory)")
	}
	return nil
}

type (
	gitFilesRequest = generated.GitFilesRequest
	changedFile     = generated.ChangedFile
)

func (s *Server) handleGitFiles(w http.ResponseWriter, r *http.Request) {
	var req gitFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.RepoPath == "" || req.LeftRef == "" || req.RightRef == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "repo_path, left_ref, and right_ref are required",
		})
		return
	}

	if err := validateRepoPath(req.RepoPath); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if !isCleanRef(req.LeftRef) || !isCleanRef(req.RightRef) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid ref"})
		return
	}

	cmd := exec.CommandContext(r.Context(), "git", "-C", req.RepoPath,
		"diff", "--name-status", req.LeftRef, req.RightRef)
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("git diff failed: %v", err),
		})
		return
	}

	var files []changedFile
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status := "modified"
		switch parts[0] {
		case "A":
			status = "added"
		case "D":
			status = "deleted"
		case "M":
			status = "modified"
		default:
			if strings.HasPrefix(parts[0], "R") {
				status = "renamed"
			}
		}
		files = append(files, changedFile{Path: parts[1], Status: generated.ChangedFileStatus(status)})
	}

	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

type (
	createChangesetRequest = generated.CreateChangesetRequest
	changesetResponse      = generated.ChangesetResponse
	fileResult             = generated.FileResult
	crossFileMatchGen      = generated.CrossFileMatch
)

func (s *Server) handleCreateChangeset(w http.ResponseWriter, r *http.Request) {
	var req createChangesetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON"})
		return
	}

	if req.RepoPath == "" || req.LeftRef == "" || req.RightRef == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "repo_path, left_ref, and right_ref are required",
		})
		return
	}

	if err := validateRepoPath(req.RepoPath); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	if !isCleanRef(req.LeftRef) || !isCleanRef(req.RightRef) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid ref"})
		return
	}

	cmd := exec.CommandContext(r.Context(), "git", "-C", req.RepoPath,
		"diff", "--name-status", req.LeftRef, req.RightRef)
	out, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: fmt.Sprintf("git diff failed: %v", err),
		})
		return
	}

	type changedEntry struct {
		path    string
		oldPath string
		status  string
	}
	var changed []changedEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		status := "modified"
		switch parts[0] {
		case "A":
			status = "added"
		case "D":
			status = "deleted"
		case "M":
			status = "modified"
		default:
			if strings.HasPrefix(parts[0], "R") {
				status = "renamed"
			}
		}
		entry := changedEntry{status: status}
		if status == "renamed" && len(parts) == 3 {
			entry.oldPath = parts[1]
			entry.path = parts[2]
		} else {
			entry.path = parts[1]
		}
		changed = append(changed, entry)
	}

	var fileDiffs []core.FileDiff
	var fileResults []fileResult
	cfg := core.DefaultMatchConfig()

	for _, ch := range changed {
		fr := fileResult{
			Path:   ch.path,
			Status: generated.FileResultStatus(ch.status),
		}

		leftPath := ch.path
		if ch.oldPath != "" {
			leftPath = ch.oldPath
		}

		var leftContent, rightContent string
		if ch.status != "added" {
			leftContent, _ = gitShow(r.Context(), req.RepoPath, req.LeftRef, leftPath)
		}
		if ch.status != "deleted" {
			rightContent, _ = gitShow(r.Context(), req.RepoPath, req.RightRef, ch.path)
		}

		ext := strings.TrimPrefix(filepath.Ext(ch.path), ".")
		lang := ""
		if p, err := s.registry.ForExtension(ext); err == nil {
			lang = p.Language()
		}
		if lang == "" {
			lang = "c"
		}
		fr.Language = lang

		parser, err := s.registry.ForLanguage(lang)
		if err != nil {
			fileResults = append(fileResults, fr)
			continue
		}

		var leftTree, rightTree *core.Tree
		if leftContent != "" {
			leftTree, _ = parser.Parse(r.Context(), []byte(leftContent))
		}
		if rightContent != "" {
			rightTree, _ = parser.Parse(r.Context(), []byte(rightContent))
		}

		if leftTree != nil && rightTree != nil {
			matching := core.Match(leftTree, rightTree, cfg)
			es := core.GenerateEditScript(leftTree, rightTree, matching)

			genES := coreEditScriptToGen(es)
			fr.EditScript = &genES

			fileDiffs = append(fileDiffs, core.FileDiff{
				Path:        ch.path,
				LeftSource:  []byte(leftContent),
				RightSource: []byte(rightContent),
				LeftTree:    leftTree,
				RightTree:   rightTree,
				Script:      es,
			})
		} else if leftTree != nil {
			es := &core.EditScript{Operations: []core.Operation{}}
			for _, child := range leftTree.Root.Children {
				ref := core.NodeRef{
					ID:    child.ID,
					Path:  child.Kind,
					Kind:  child.Kind,
					Label: child.Label,
					Location: core.Location{
						Line:   child.Span.Start.Line,
						Column: child.Span.Start.Column,
						Offset: child.Span.Start.Offset,
					},
				}
				es.Operations = append(es.Operations, core.Operation{
					Kind:     core.OpDelete,
					LeftNode: &ref,
				})
			}
			genES := coreEditScriptToGen(es)
			fr.EditScript = &genES

			fileDiffs = append(fileDiffs, core.FileDiff{
				Path:       ch.path,
				LeftSource: []byte(leftContent),
				LeftTree:   leftTree,
				Script:     es,
			})
		} else if rightTree != nil {
			es := &core.EditScript{Operations: []core.Operation{}}
			for _, child := range rightTree.Root.Children {
				ref := core.NodeRef{
					ID:    child.ID,
					Path:  child.Kind,
					Kind:  child.Kind,
					Label: child.Label,
					Location: core.Location{
						Line:   child.Span.Start.Line,
						Column: child.Span.Start.Column,
						Offset: child.Span.Start.Offset,
					},
				}
				es.Operations = append(es.Operations, core.Operation{
					Kind:      core.OpInsert,
					RightNode: &ref,
				})
			}
			genES := coreEditScriptToGen(es)
			fr.EditScript = &genES

			fileDiffs = append(fileDiffs, core.FileDiff{
				Path:        ch.path,
				RightSource: []byte(rightContent),
				RightTree:   rightTree,
				Script:      es,
			})
		}

		fileResults = append(fileResults, fr)
	}

	crossFileResult := core.DetectCrossFileChanges(fileDiffs)
	var cfMatches []crossFileMatchGen
	for _, m := range crossFileResult.Matches {
		cfMatches = append(cfMatches, crossFileMatchGen{
			Kind:       generated.CrossFileMatchKind(m.Kind),
			Score:      float32(m.Score),
			SourceFile: m.SourceFile,
			TargetFile: m.TargetFile,
			SourceNode: *toNodeRefPtr(&m.SourceNode),
			TargetNode: *toNodeRefPtr(&m.TargetNode),
		})
	}

	if fileResults == nil {
		fileResults = []fileResult{}
	}
	if cfMatches == nil {
		cfMatches = []crossFileMatchGen{}
	}

	writeJSON(w, http.StatusOK, changesetResponse{
		Files:            fileResults,
		CrossFileMatches: cfMatches,
	})
}

func coreEditScriptToGen(es *core.EditScript) generated.EditScript {
	ops := make([]generated.Operation, len(es.Operations))
	for i, op := range es.Operations {
		ops[i] = generated.Operation{
			Kind:      generated.OperationKind(op.Kind),
			LeftNode:  toNodeRefPtr(op.LeftNode),
			RightNode: toNodeRefPtr(op.RightNode),
		}
	}
	genES := generated.EditScript{
		Operations: ops,
		LeftRoot:   int(es.LeftRoot),
		RightRoot:  int(es.RightRoot),
	}
	if len(es.Approximate) > 0 {
		approx := make([]generated.ApproximateRegion, len(es.Approximate))
		for i, r := range es.Approximate {
			approx[i] = generated.ApproximateRegion{
				LeftSpan:  spanToGen(r.LeftSpan),
				RightSpan: spanToGen(r.RightSpan),
			}
		}
		genES.Approximate = &approx
	}
	if len(es.Semantic) > 0 {
		sem := make([]generated.SemanticChange, len(es.Semantic))
		for i, sc := range es.Semantic {
			sem[i] = generated.SemanticChange{
				LeftNode:  *toNodeRefPtr(sc.LeftNode),
				RightNode: *toNodeRefPtr(sc.RightNode),
				Verdict:   generated.SemanticChangeVerdict(sc.Verdict),
				Reason:    sc.Reason,
			}
		}
		genES.Semantic = &sem
	}
	return genES
}

func spanToGen(s core.Span) generated.Span {
	return generated.Span{
		Start: generated.Location{
			Line:   s.Start.Line,
			Column: s.Start.Column,
			Offset: s.Start.Offset,
		},
		End: generated.Location{
			Line:   s.End.Line,
			Column: s.End.Column,
			Offset: s.End.Offset,
		},
	}
}

func detectLanguage(left, right string, s *Server) string {
	for _, filename := range []string{left, right} {
		if filename == "" {
			continue
		}
		ext := strings.TrimPrefix(filepath.Ext(filename), ".")
		if ext == "" {
			continue
		}
		p, err := s.registry.ForExtension(ext)
		if err == nil {
			return p.Language()
		}
	}
	return ""
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
