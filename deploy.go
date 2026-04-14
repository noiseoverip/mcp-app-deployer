package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/mark3labs/mcp-go/mcp"
)

//go:embed templates/*
var templatesFS embed.FS

func deploy(ctx context.Context, appName, image, exposure, targetNamespace string) (*mcp.CallToolResult, error) {
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
	data := ImageManifestData{
		Name:         appName,
		Image:        image,
		Namespace:    targetNamespace,
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

	argoData := ArgoApplicationData{
		Name:      appName,
		Namespace: targetNamespace,
		Git: &ArgoGitSource{
			RepoURL:        githubURL,
			TargetRevision: "HEAD",
			Path:           filepath.ToSlash(filepath.Join(manifestPath, appName)),
		},
	}

	if err := writeArgoApplication(tempDir, w, argocdPath, "templates/application.yaml", argoData); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to render argo app: %v", err)), nil
	}

	if exposure == exposureLocal {
		if err := ensureLocalNamespaceNetworkPolicy(tempDir, w, targetNamespace); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to render local network policy: %v", err)), nil
		}
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

// ensureLocalNamespaceNetworkPolicy writes a NetworkPolicy manifest and an
// ArgoCD Application that deploys it into the local namespace, restricting
// ingress to the configured subnets. The files are idempotent: rewriting them
// with the same content produces no git diff.
func ensureLocalNamespaceNetworkPolicy(tempDir string, w *git.Worktree, localNs string) error {
	subnets := allowedSubnets()
	if len(subnets) == 0 {
		return fmt.Errorf("--local-allowed-subnets must be set to deploy applications with exposure=local")
	}

	bootstrapName := "local-namespace-network-policy"
	npDir := filepath.Join(manifestPath, bootstrapName)
	npDirAbs := filepath.Join(tempDir, npDir)
	if err := os.MkdirAll(npDirAbs, 0755); err != nil {
		return fmt.Errorf("create network policy dir: %w", err)
	}

	tmpl, err := template.ParseFS(templatesFS, "templates/networkpolicy.yaml")
	if err != nil {
		return fmt.Errorf("parse networkpolicy template: %w", err)
	}

	npFile := filepath.Join(npDirAbs, "networkpolicy.yaml")
	f, err := os.Create(npFile)
	if err != nil {
		return fmt.Errorf("create networkpolicy file: %w", err)
	}
	defer f.Close()

	npData := struct {
		Namespace      string
		AllowedSubnets []string
	}{Namespace: localNs, AllowedSubnets: subnets}

	if err := tmpl.Execute(f, npData); err != nil {
		return fmt.Errorf("execute networkpolicy template: %w", err)
	}

	if _, err := w.Add(filepath.ToSlash(filepath.Join(npDir, "networkpolicy.yaml"))); err != nil {
		return fmt.Errorf("git add networkpolicy: %w", err)
	}

	argoData := ArgoApplicationData{
		Name:      bootstrapName,
		Namespace: localNs,
		Git: &ArgoGitSource{
			RepoURL:        githubURL,
			TargetRevision: "HEAD",
			Path:           filepath.ToSlash(npDir),
		},
	}

	argocdPath := filepath.Join(tempDir, argocdAppPath)
	return writeArgoApplication(tempDir, w, argocdPath, "templates/application.yaml", argoData)
}

func writeArgoApplication(tempDir string, w *git.Worktree, argocdPath string, templatePath string, data ArgoApplicationData) error {
	tmpl, err := template.ParseFS(templatesFS, templatePath)
	if err != nil {
		return fmt.Errorf("parse application template: %w", err)
	}

	argoAppFile := filepath.Join(argocdPath, data.Name+".yaml")
	f, err := os.Create(argoAppFile)
	if err != nil {
		return fmt.Errorf("create argo app file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute application template: %w", err)
	}

	relativeAppFile, err := filepath.Rel(tempDir, argoAppFile)
	if err != nil {
		return fmt.Errorf("compute argo app path: %w", err)
	}

	if _, err := w.Add(filepath.ToSlash(relativeAppFile)); err != nil {
		return fmt.Errorf("git add argo app: %w", err)
	}

	return nil
}
