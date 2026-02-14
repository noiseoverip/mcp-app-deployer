package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/mark3labs/mcp-go/mcp"
)

//go:embed templates/*
var templatesFS embed.FS

func deploy(ctx context.Context, appName, image string) (*mcp.CallToolResult, error) {
	// 1. Clone the repository
	tempDir, err := os.MkdirTemp("", "mcp-deployer-")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp dir: %v", err)), nil
	}
	defer os.RemoveAll(tempDir)

	auth := &http.BasicAuth{
		Username: "oauth2", // Common for tokens
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

	// 2. Prepare paths
	appManifestPath := filepath.Join(tempDir, manifestPath, appName)
	if err := os.MkdirAll(appManifestPath, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create manifest dir: %v", err)), nil
	}

	argocdPath := filepath.Join(tempDir, argocdAppPath)
	if err := os.MkdirAll(argocdPath, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create argocd app dir: %v", err)), nil
	}

	// 3. Render templates
	data := struct {
		Name         string
		Image        string
		Namespace    string
		Domain       string
		RepoURL      string
		ManifestPath string
	}{
		Name:         appName,
		Image:        image,
		Namespace:    namespace,
		Domain:       domain,
		RepoURL:      githubURL,
		ManifestPath: manifestPath,
	}

	// Render Kubernetes Manifests
	manifests := []string{"deployment.yaml", "service.yaml", "ingress.yaml"}
	for _, tmplName := range manifests {
		tmpl, err := template.ParseFS(templatesFS, "templates/"+tmplName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse template %s: %v", tmplName, err)), nil
		}

		f, err := os.Create(filepath.Join(appManifestPath, tmplName))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create file %s: %v", tmplName, err)), nil
		}
		defer f.Close()

		if err := tmpl.Execute(f, data); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to execute template %s: %v", tmplName, err)), nil
		}

		if _, err := w.Add(filepath.Join(manifestPath, appName, tmplName)); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to git add %s: %v", tmplName, err)), nil
		}
	}

	// Render ArgoCD Application
	tmpl, err := template.ParseFS(templatesFS, "templates/application.yaml")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse application template: %v", err)), nil
	}

	argoAppFile := filepath.Join(argocdPath, appName+".yaml")
	f, err := os.Create(argoAppFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create argo app file: %v", err)), nil
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to execute application template: %v", err)), nil
	}

	if _, err := w.Add(filepath.Join(argocdAppPath, appName+".yaml")); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to git add argo app: %v", err)), nil
	}

	// 4. Commit and Push
	status, err := w.Status()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get git status: %v", err)), nil
	}

	if status.IsClean() {
		return mcp.NewToolResultText("No changes to deploy"), nil
	}

	commitMsg := fmt.Sprintf("Deploy application %s with image %s", appName, image)
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

	return mcp.NewToolResultText(fmt.Sprintf("Successfully deployed %s. Git updated.", appName)), nil
}
