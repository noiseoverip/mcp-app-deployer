package main

import (
	"context"
	"crypto/subtle"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	kubeconfigPath      string
	namespace           string
	localNamespace      string
	localAllowedSubnets string
	domain              string
	githubURL           string
	githubToken         string
	argocdAppPath       string
	manifestPath        string
	httpAddr            string
	httpPassword        string
	httpEndpointPath    string
)

const (
	exposurePublic = "public"
	exposureLocal  = "local"
)

func resolveExposure(args map[string]interface{}) (string, string, error) {
	raw, ok := args["exposure"]
	if !ok || raw == nil {
		return exposurePublic, namespace, nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", "", fmt.Errorf("exposure must be a string")
	}
	switch str {
	case "", exposurePublic:
		return exposurePublic, namespace, nil
	case exposureLocal:
		return exposureLocal, localNamespace, nil
	default:
		return "", "", fmt.Errorf("exposure must be %q or %q", exposurePublic, exposureLocal)
	}
}

func allowedSubnets() []string {
	if strings.TrimSpace(localAllowedSubnets) == "" {
		return nil
	}
	parts := strings.Split(localAllowedSubnets, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func main() {
	// Parse command line flags
	flag.StringVar(&kubeconfigPath, "kubeconfig", "", "Path to kubeconfig file")
	flag.StringVar(&namespace, "namespace", "applications", "Kubernetes namespace for public applications")
	flag.StringVar(&localNamespace, "local-namespace", "applications-local", "Kubernetes namespace for local applications")
	flag.StringVar(&localAllowedSubnets, "local-allowed-subnets", "", "Comma-separated list of CIDR subnets permitted to reach local applications")
	flag.StringVar(&domain, "domain", "tykus.net", "Base domain for ingress")
	flag.StringVar(&githubURL, "github-url", "", "GitHub URL (e.g., https://github.com/user/repo)")
	flag.StringVar(&githubToken, "github-token", "", "GitHub Personal Access Token")
	flag.StringVar(&argocdAppPath, "argocd-path", "argocd-apps", "Path in repo for ArgoCD apps")
	flag.StringVar(&manifestPath, "manifest-path", "manifests", "Path in repo for Kubernetes manifests")
	flag.StringVar(&httpAddr, "http", "", "If set (e.g. \":8080\"), serve MCP over Streamable HTTP on this address instead of stdio")
	flag.StringVar(&httpPassword, "http-password", "", "Bearer token required to access the HTTP endpoint (required when --http is set; falls back to MCP_HTTP_PASSWORD env var)")
	flag.StringVar(&httpEndpointPath, "http-path", "/mcp", "URL path for the MCP HTTP endpoint")
	flag.Parse()

	if httpAddr != "" && httpPassword == "" {
		httpPassword = os.Getenv("MCP_HTTP_PASSWORD")
	}

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
	s.AddTool(mcp.NewTool("deploy-image",
		mcp.WithDescription("Deploy a new application from a container image"),
		mcp.WithString("app_name", mcp.Required(), mcp.Description("Name of the application")),
		mcp.WithString("image", mcp.Required(), mcp.Description("Container image to deploy")),
		mcp.WithString("exposure", mcp.Description("Exposure mode: \"public\" (default) exposes the app to the public internet; \"local\" restricts it to configured local subnets")),
	), deployHandler)

	s.AddTool(mcp.NewTool("deploy-helmchart",
		mcp.WithDescription("Deploy a new application from an OCI Helm chart"),
		mcp.WithString("app_name", mcp.Required(), mcp.Description("Name of the application")),
		mcp.WithString("chart", mcp.Required(), mcp.Description("Full OCI Helm chart reference including version, for example oci://registry-1.docker.io/bitnamicharts/nginx:15.9.0")),
		mcp.WithString("exposure", mcp.Description("Exposure mode: \"public\" (default) exposes the app to the public internet; \"local\" restricts it to configured local subnets")),
	), deployHelmChartHandler)

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

	if httpAddr != "" {
		if httpPassword == "" {
			fmt.Println("Error: --http-password (or MCP_HTTP_PASSWORD) is required when --http is set")
			os.Exit(1)
		}
		httpServer := server.NewStreamableHTTPServer(s,
			server.WithEndpointPath(httpEndpointPath),
		)
		mux := http.NewServeMux()
		mux.Handle(httpEndpointPath, authMiddleware(httpPassword, httpServer))
		srv := &http.Server{Addr: httpAddr, Handler: mux}
		log.Printf("MCP HTTP server listening on %s%s", httpAddr, httpEndpointPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v\n", err)
		}
		return
	}

	// Start server on stdio
	if err := server.ServeStdio(s); err != nil {
		log.Printf("Server error: %v\n", err)
	}
}

func authMiddleware(password string, next http.Handler) http.Handler {
	expected := []byte(password)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var provided string
		if h := r.Header.Get("Authorization"); h != "" {
			if strings.HasPrefix(h, "Bearer ") {
				provided = strings.TrimPrefix(h, "Bearer ")
			}
		}
		if provided == "" {
			provided = r.Header.Get("X-MCP-Password")
		}
		if subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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

	exposure, targetNamespace, err := resolveExposure(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return deploy(ctx, appName, image, exposure, targetNamespace)
}

func deployHelmChartHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("arguments must be a map"), nil
	}

	appName, ok := args["app_name"].(string)
	if !ok {
		return mcp.NewToolResultError("app_name must be a string"), nil
	}
	chartRef, ok := args["chart"].(string)
	if !ok {
		return mcp.NewToolResultError("chart must be a string"), nil
	}

	exposure, targetNamespace, err := resolveExposure(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return deployHelmChart(ctx, appName, chartRef, exposure, targetNamespace)
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
