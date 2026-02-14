package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	kubeconfigPath string
	namespace      string
	domain         string
	githubURL      string
	githubToken    string
	argocdAppPath  string
	manifestPath   string
)

func main() {
	// Parse command line flags
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	flag.StringVar(&namespace, "namespace", "applications", "Kubernetes namespace")
	flag.StringVar(&domain, "domain", "tykus.net", "Base domain for ingress")
	flag.StringVar(&githubURL, "github-url", "", "GitHub URL (e.g., https://github.com/user/repo)")
	flag.StringVar(&githubToken, "github-token", "", "GitHub Personal Access Token")
	flag.StringVar(&argocdAppPath, "argocd-path", "argocd-apps", "Path in repo for ArgoCD apps")
	flag.StringVar(&manifestPath, "manifest-path", "manifests", "Path in repo for Kubernetes manifests")
	flag.Parse()

	// Validate required flags
	if kubeconfigPath == "" || githubURL == "" || githubToken == "" {
		fmt.Println("Error: --kubeconfig, --github-url, and --github-token are required")
		flag.Usage()
		os.Exit(1)
	}

	// Create MCP server
	s := server.NewMCPServer(
		"mcp-app-deployer",
		"1.0.0",
		server.WithLogging(),
	)

	// Register tools
	s.AddTool(mcp.NewTool("deploy",
		mcp.WithDescription("Deploy a new application"),
		mcp.WithString("app_name", mcp.Required(), mcp.Description("Name of the application")),
		mcp.WithString("image", mcp.Required(), mcp.Description("Container image to deploy")),
	), deployHandler)

	s.AddTool(mcp.NewTool("destroy",
		mcp.WithDescription("Destroy an existing application"),
		mcp.WithString("app_name", mcp.Required(), mcp.Description("Name of the application")),
	), destroyHandler)

	s.AddTool(mcp.NewTool("status",
		mcp.WithDescription("Get status of an application"),
		mcp.WithString("app_name", mcp.Required(), mcp.Description("Name of the application")),
	), statusHandler)

	s.AddTool(mcp.NewTool("update",
		mcp.WithDescription("Trigger a rolling restart of an application's deployment"),
		mcp.WithString("app_name", mcp.Required(), mcp.Description("Name of the application")),
	), updateHandler)

	// Start server on stdio
	if err := server.ServeStdio(s); err != nil {
		log.Printf("Server error: %v\n", err)
	}
}

func deployHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("arguments must be a map"), nil
	}

	appName, ok := args["app_name"].(string)
	if !ok {
		return mcp.NewToolResultError("app_name must be a string"), nil
	}
	image, ok := args["image"].(string)
	if !ok {
		return mcp.NewToolResultError("image must be a string"), nil
	}

	return deploy(ctx, appName, image)
}

func destroyHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("arguments must be a map"), nil
	}

	appName, ok := args["app_name"].(string)
	if !ok {
		return mcp.NewToolResultError("app_name must be a string"), nil
	}

	return destroy(ctx, appName)
}

func statusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("arguments must be a map"), nil
	}

	appName, ok := args["app_name"].(string)
	if !ok {
		return mcp.NewToolResultError("app_name must be a string"), nil
	}

	return status(ctx, appName)
}

func updateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("arguments must be a map"), nil
	}

	appName, ok := args["app_name"].(string)
	if !ok {
		return mcp.NewToolResultError("app_name must be a string"), nil
	}

	return update(ctx, appName)
}
