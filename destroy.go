package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/mark3labs/mcp-go/mcp"
)

func destroy(ctx context.Context, appName string) (*mcp.CallToolResult, error) {
	// 1. Clone the repository
	tempDir, err := os.MkdirTemp("", "mcp-destroyer-")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp dir: %v", err)), nil
	}
	defer os.RemoveAll(tempDir)

	auth := &http.BasicAuth{
		Username: "oauth2",
		Password: githubToken,
	}

	repo, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:  githubURL,
		Auth: auth,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to clone repo: %v", err)), nil
	}

	w, err := repo.Worktree()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get worktree: %v", err)), nil
	}

	// 2. Remove files
	manifestDir := filepath.Join(manifestPath, appName)
	if _, err := w.Filesystem.Stat(manifestDir); err == nil {
		if _, err := w.Remove(manifestDir); err != nil {
			// Try recursive removal if directory
			if err := os.RemoveAll(filepath.Join(tempDir, manifestDir)); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to remove manifest dir: %v", err)), nil
			}
			// Add the removal to git index
			if _, err := w.Add(manifestPath); err != nil {
				// If adding the parent dir fails, we might need to be more specific or use `w.Remove` correctly on files.
				// For simplicity, let's try `git rm -r` equivalent.
				// Since go-git `Remove` is file-based, removing a directory can be tricky.
				// The easiest way is to remove from filesystem and then `w.Add(".")`.
			}
		}
	} else {
		// Does not exist, ignore
	}

	// Clean up robustly: delete from FS, then Add(all)
	if err := os.RemoveAll(filepath.Join(tempDir, manifestDir)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to remove manifest dir from FS: %v", err)), nil
	}

	argoAppFile := filepath.Join(argocdAppPath, appName+".yaml")
	if err := os.Remove(filepath.Join(tempDir, argoAppFile)); err != nil && !os.IsNotExist(err) {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to remove argo app file: %v", err)), nil
	}

	// Add changes to index (including deletions)
	if _, err := w.Add("."); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to stage changes: %v", err)), nil
	}

	// 3. Commit and Push
	status, err := w.Status()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get git status: %v", err)), nil
	}

	if status.IsClean() {
		return mcp.NewToolResultText(fmt.Sprintf("App %s does not exist or already destroyed", appName)), nil
	}

	commitMsg := fmt.Sprintf("Destroy application %s", appName)
	_, err = w.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "MCP App Deployer",
			Email: "mcp-deployer@bot.local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to commit changes: %v", err)), nil
	}

	if err := repo.Push(&git.PushOptions{Auth: auth}); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to push changes: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully destroyed %s (manifests removed).", appName)), nil
}
