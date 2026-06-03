package gitbackend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	dataDir string
}

type TreeEntry struct {
	Name    string
	Mode    string
	Type    string
	Size    int64
	SHA     string
	IsDir   bool
}

type Commit struct {
	SHA       string
	Message   string
	Author    string
	Email     string
	Timestamp time.Time
	Parents   []string
}

type DiffFile struct {
	OldName string
	NewName string
	Status  string
	Patch   string
}

func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

func (s *Service) RepoPath(owner, name string) string {
	return filepath.Join(s.dataDir, owner, name+".git")
}

func (s *Service) AbsPath(diskPath string) string {
	if filepath.IsAbs(diskPath) {
		return diskPath
	}
	return filepath.Join(s.dataDir, diskPath)
}

func (s *Service) InitBare(diskPath, defaultBranch string) error {
	absPath := s.AbsPath(diskPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}

	cmd := exec.Command("git", "init", "--bare", absPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %s: %w", string(out), err)
	}

	if defaultBranch != "" && defaultBranch != "master" {
		cmd = exec.Command("git", "-C", absPath, "symbolic-ref", "HEAD", "refs/heads/"+defaultBranch)
		cmd.Run()
	}

	return nil
}

func (s *Service) Delete(diskPath string) error {
	absPath := s.AbsPath(diskPath)
	return os.RemoveAll(absPath)
}

func (s *Service) Rename(oldPath, newPath string) error {
	absOld := s.AbsPath(oldPath)
	absNew := s.AbsPath(newPath)
	if err := os.MkdirAll(filepath.Dir(absNew), 0755); err != nil {
		return err
	}
	return os.Rename(absOld, absNew)
}

func (s *Service) Exists(diskPath string) bool {
	absPath := s.AbsPath(diskPath)
	info, err := os.Stat(absPath)
	return err == nil && info.IsDir()
}

func (s *Service) ListBranches(diskPath string) ([]string, error) {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "branch", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}

func (s *Service) ListTags(diskPath string) ([]string, error) {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "tag", "--sort=-creatordate")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			tags = append(tags, line)
		}
	}
	return tags, nil
}

func (s *Service) DefaultRef(diskPath string) string {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "symbolic-ref", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(out))
}

func (s *Service) ResolveRef(diskPath, ref string) (string, error) {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "rev-parse", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot resolve ref %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) GetTree(diskPath, ref, path string) ([]TreeEntry, error) {
	absPath := s.AbsPath(diskPath)

	treeRef := ref
	if path != "" {
		treeRef = ref + ":" + path
	}

	cmd := exec.Command("git", "-C", absPath, "ls-tree", "-l", treeRef)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var entries []TreeEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		entry := parseTreeLine(line)
		entries = append(entries, entry)
	}

	sortTreeEntries(entries)
	return entries, nil
}

func parseTreeLine(line string) TreeEntry {
	parts := strings.Fields(line)
	if len(parts) < 5 {
		return TreeEntry{Name: line}
	}

	mode := parts[0]
	objType := parts[1]
	sha := parts[2]
	name := parts[4]

	var size int64
	if parts[3] != "-" {
		fmt.Sscanf(parts[3], "%d", &size)
	}

	if len(parts) > 5 {
		name = strings.Join(parts[4:], " ")
	}

	return TreeEntry{
		Name:  name,
		Mode:  mode,
		Type:  objType,
		SHA:   sha,
		Size:  size,
		IsDir: objType == "tree",
	}
}

func sortTreeEntries(entries []TreeEntry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if !entries[i].IsDir && entries[j].IsDir {
				entries[i], entries[j] = entries[j], entries[i]
			} else if entries[i].IsDir == entries[j].IsDir && entries[i].Name > entries[j].Name {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

func (s *Service) GetBlob(diskPath, ref, path string) ([]byte, error) {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "show", ref+":"+path)
	return cmd.Output()
}

func (s *Service) GetCommit(diskPath, sha string) (*Commit, error) {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "log", "-1",
		"--format=%H%n%s%n%an%n%ae%n%aI%n%P", sha)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 5 {
		return nil, fmt.Errorf("unexpected git log output")
	}

	ts, _ := time.Parse(time.RFC3339, lines[4])
	commit := &Commit{
		SHA:       lines[0],
		Message:   lines[1],
		Author:    lines[2],
		Email:     lines[3],
		Timestamp: ts,
	}
	if len(lines) > 5 && lines[5] != "" {
		commit.Parents = strings.Fields(lines[5])
	}
	return commit, nil
}

func (s *Service) ListCommits(diskPath, ref string, page, limit int) ([]*Commit, error) {
	absPath := s.AbsPath(diskPath)
	skip := (page - 1) * limit

	cmd := exec.Command("git", "-C", absPath, "log",
		fmt.Sprintf("--skip=%d", skip),
		fmt.Sprintf("--max-count=%d", limit),
		"--format=%H||%s||%an||%ae||%aI",
		ref)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []*Commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "||", 5)
		if len(parts) < 5 {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, parts[4])
		commits = append(commits, &Commit{
			SHA:       parts[0],
			Message:   parts[1],
			Author:    parts[2],
			Email:     parts[3],
			Timestamp: ts,
		})
	}
	return commits, nil
}

func (s *Service) GetDiff(diskPath, sha string) ([]*DiffFile, error) {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "diff-tree", "-p", "--no-commit-id", sha)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseDiff(string(out)), nil
}

func parseDiff(raw string) []*DiffFile {
	var files []*DiffFile
	chunks := strings.Split(raw, "diff --git ")
	for _, chunk := range chunks[1:] {
		lines := strings.SplitN(chunk, "\n", 2)
		if len(lines) < 2 {
			continue
		}
		header := lines[0]
		parts := strings.Fields(header)
		var oldName, newName string
		if len(parts) >= 2 {
			oldName = strings.TrimPrefix(parts[0], "a/")
			newName = strings.TrimPrefix(parts[1], "b/")
		}
		files = append(files, &DiffFile{
			OldName: oldName,
			NewName: newName,
			Patch:   lines[1],
		})
	}
	return files
}

func (s *Service) IsEmpty(diskPath string) bool {
	absPath := s.AbsPath(diskPath)
	cmd := exec.Command("git", "-C", absPath, "rev-parse", "--verify", "HEAD")
	err := cmd.Run()
	return err != nil
}

func (s *Service) GetReadme(diskPath, ref string) (string, error) {
	absPath := s.AbsPath(diskPath)
	readmeNames := []string{"README.md", "readme.md", "README", "README.txt"}
	for _, name := range readmeNames {
		cmd := exec.Command("git", "-C", absPath, "show", ref+":"+name)
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}
	return "", nil
}
