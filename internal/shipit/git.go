package shipit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GitStatus(repoPath string) ([]string, error) {
	out, err := git(repoPath, "status", "--short")
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(out)
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func GitDiff(repoPath string) (string, error) {
	return git(repoPath, "diff", "--binary")
}

func GitDiffNameOnly(repoPath string) ([]string, error) {
	out, err := git(repoPath, "diff", "--name-only")
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(out)
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func GitChangedPaths(repoPath string) ([]string, error) {
	out, err := git(repoPath, "status", "--short")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 4 {
			continue
		}
		paths = append(paths, strings.TrimSpace(line[3:]))
	}
	return paths, nil
}

func GitUntrackedPaths(repoPath string) ([]string, error) {
	out, err := git(repoPath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(out)
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func GitHasChanges(repoPath string) (bool, error) {
	status, err := GitStatus(repoPath)
	if err != nil {
		return false, err
	}
	return len(status) > 0, nil
}

func GitCommitAll(repoPath, message string) (string, error) {
	hasChanges, err := GitHasChanges(repoPath)
	if err != nil {
		return "", err
	}
	if !hasChanges {
		return "", nil
	}
	if _, err := git(repoPath, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := git(repoPath, "commit", "-m", message); err != nil {
		return "", err
	}
	sha, err := git(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func GitResetHard(repoPath, ref string) error {
	if ref == "" {
		ref = "HEAD"
	}
	if _, err := git(repoPath, "reset", "--hard", ref); err != nil {
		return err
	}
	_, err := git(repoPath, "clean", "-fd")
	return err
}

func GitLogOneline(repoPath string, max int) ([]string, error) {
	args := []string{"log", "--oneline"}
	if max > 0 {
		args = append(args, fmt.Sprintf("-%d", max))
	}
	out, err := git(repoPath, args...)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(out)
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func IsGitRepo(repoPath string) error {
	out, err := git(repoPath, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "true" {
		return errors.New("not inside a git work tree")
	}
	return nil
}

func CreateWorktree(repoPath, workspaceDir, branch string) error {
	if err := os.MkdirAll(filepath.Dir(workspaceDir), 0755); err != nil {
		return err
	}
	_, err := git(repoPath, "worktree", "add", "-b", branch, workspaceDir, "HEAD")
	if err != nil {
		return err
	}
	return nil
}

func EnsureShipitIgnored(repoPath string) error {
	excludePath := filepath.Join(repoPath, ".git", "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "\n.shipit/\n") || strings.HasSuffix(string(data), "\n.shipit/") {
		return nil
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(data) > 0 && data[len(data)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(".shipit/\n")
	return err
}

func git(repoPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
