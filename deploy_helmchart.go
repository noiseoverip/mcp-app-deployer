package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/mark3labs/mcp-go/mcp"
)

func deployHelmChart(ctx context.Context, appName, chartRef string) (*mcp.CallToolResult, error) {
	chartSource, err := parseOCIHelmChartRef(chartRef)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid OCI chart reference: %v", err)), nil
	}

	host := fmt.Sprintf("%s.%s", appName, domain)

	tempDir, err := os.MkdirTemp("", "mcp-helm-deployer-")
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

	argocdPath := filepath.Join(tempDir, argocdAppPath)
	if err := os.MkdirAll(argocdPath, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create argocd app dir: %v", err)), nil
	}

	argoData := ArgoApplicationData{
		Name:      appName,
		Namespace: namespace,
		Helm: &ArgoHelmSource{
			RepoURL:        chartSource.RepoURL,
			Chart:          chartSource.Chart,
			TargetRevision: chartSource.TargetRevision,
			ReleaseName:    appName,
			IngressName:    appName,
			IngressHost:    host,
		},
	}

	if err := writeArgoApplication(tempDir, w, argocdPath, "templates/application-helm.yaml", argoData); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to render argo app: %v", err)), nil
	}

	status, err := w.Status()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get git status: %v", err)), nil
	}

	if status.IsClean() {
		return mcp.NewToolResultText("No changes to deploy"), nil
	}

	commitMsg := fmt.Sprintf("Deploy application %s with Helm chart %s", appName, chartRef)
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

	if err := waitForArgoApplicationHealthy(ctx, appName, 3*time.Minute, 5*time.Second); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Git updated but ArgoCD did not become ready: %v", err)), nil
	}

	if err := waitForIngressReachability(ctx, host, 2*time.Minute, 5*time.Second); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("ArgoCD synced %s but ingress did not become reachable: %v", appName, err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully deployed %s from Helm chart %s. ArgoCD is synced and %s is reachable.", appName, chartRef, host)), nil
}

func parseOCIHelmChartRef(chartRef string) (*ArgoHelmSource, error) {
	trimmed := strings.TrimSpace(chartRef)
	if trimmed == "" {
		return nil, fmt.Errorf("chart reference is empty")
	}

	trimmed = strings.TrimPrefix(trimmed, "oci://")
	lastSlash := strings.LastIndex(trimmed, "/")
	lastColon := strings.LastIndex(trimmed, ":")

	if lastSlash <= 0 || lastColon <= lastSlash+1 || lastColon == len(trimmed)-1 {
		return nil, fmt.Errorf("expected format oci://registry/path/chart:version")
	}

	chart := trimmed[lastSlash+1 : lastColon]
	repoURL := trimmed[:lastSlash]
	targetRevision := trimmed[lastColon+1:]

	if chart == "" || repoURL == "" || targetRevision == "" {
		return nil, fmt.Errorf("expected format oci://registry/path/chart:version")
	}

	return &ArgoHelmSource{
		RepoURL:        repoURL,
		Chart:          chart,
		TargetRevision: targetRevision,
	}, nil
}
