package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func status(ctx context.Context, appName string) (*mcp.CallToolResult, error) {
	result := []string{fmt.Sprintf("Status for application: %s", appName)}

	// 1. Check Git Status
	existsInGit, err := checkGitStatus(appName)
	if err != nil {
		result = append(result, fmt.Sprintf("Error checking git: %v", err))
	} else if existsInGit {
		result = append(result, "✅ Manifests present in Git")
	} else {
		result = append(result, "❌ Manifests NOT found in Git")
	}

	// 2. Check ArgoCD App Status
	// We'll look for the Application CR in the "argocd" namespace (or wherever ArgoCD is installed)
	// The user didn't specify ArgoCD namespace, but conventionally it is `argocd`.
	// The spec says "Wait for expected ArgoCD application to appear in Kubernetes cluster"
	argocdStatus, err := checkArgoStatus(ctx, appName)
	if err != nil {
		result = append(result, fmt.Sprintf("Error checking ArgoCD: %v", err))
	} else {
		result = append(result, argocdStatus)
	}

	// 3. Check Ingress Reachability
	ingressURL := fmt.Sprintf("http://%s.%s", appName, domain)
	reachable := checkReachability(ingressURL)
	if reachable {
		result = append(result, fmt.Sprintf("✅ Ingress reachable: %s", ingressURL))
	} else {
		result = append(result, fmt.Sprintf("❌ Ingress unreachable: %s", ingressURL))
	}

	return mcp.NewToolResultText(strings.Join(result, "\n")), nil
}

func checkGitStatus(appName string) (bool, error) {
	// We use an in-memory clone for speed, just checking existence
	auth := &githttp.BasicAuth{
		Username: "oauth2",
		Password: githubToken,
	}

	r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:   githubURL,
		Auth:  auth,
		Depth: 1,
	})
	if err != nil {
		return false, err
	}

	// Check if manifest file exists in the HEAD
	ref, err := r.Head()
	if err != nil {
		return false, err
	}

	commit, err := r.CommitObject(ref.Hash())
	if err != nil {
		return false, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return false, err
	}

	// Check for application.yaml in argo path
	argoPath := fmt.Sprintf("%s/%s.yaml", argocdAppPath, appName)
	_, err = tree.File(argoPath)
	return err == nil, nil
}

func checkArgoStatus(ctx context.Context, appName string) (string, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "", err
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return "", err
	}

	// Define ArgoCD Application GVR
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	// Assuming ArgoCD is in 'argocd' namespace.
	// In a real implementation we might want this configurable.
	app, err := dynClient.Resource(gvr).Namespace("argocd").Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return fmt.Sprintf("❌ ArgoCD Application not found: %v", err), nil
	}

	// Parse status
	// Status is unstructured, simplified check
	status, found, _ := unstructured.NestedString(app.Object, "status", "health", "status")
	if found {
		syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
		return fmt.Sprintf("✅ ArgoCD App found. Health: %s, Sync: %s", status, syncStatus), nil
	}

	return "⚠️ ArgoCD App found but status unknown", nil
}

func checkReachability(url string) bool {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
