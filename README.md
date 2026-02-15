# MCP server for deploying to Kubernetes

I use this for my home lab setup to improve agenting development loop with real deployment and testing.
Give it app name and container image name and it will generate Kubernetes manifests and push them to git, additional action let agent check status, restart with latest image and destroy everything.


## Prerequisites

- A Kubernetes cluster (and `kubeconfig`)
- A GitHub repository for GitOps
- ArgoCD installed on the cluster
- `GITHUB_TOKEN` with repo access

## Installation

1.  Build the server:
    ```bash
    go build -o app-deployer .
    ```

## Configuration

Add the server to your MCP client configuration (e.g., `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "app-deployer": {
      "command": "/absolute/path/to/app-deployer",
      "args": [
        "--kubeconfig", "/path/to/kubeconfig",
        "--github-token", "YOUR_GITHUB_TOKEN",
        "--github-url", "https://github.com/your/repo",
        "--namespace", "applications", // Target namespace for apps
        "--domain", "example.com" // Base domain for Ingress
      ]
    }
  }
}
```

## Usage

### 1. Deploy an Application

Use the deploy tool to create a new deployment.

**Tool:** deploy
**Arguments:**
- `app_name`: "my-app"
- `image`: "nginx:latest"

This will:
- Generate Kubernetes manifests in Git.
- Create an ArgoCD Application in Git.
- Push changes to the repository. ArgoCD should then sync the app.

### 2. Check Status

Use the status tool to check if the application is deployed and healthy.

**Tool:** status
**Arguments:**
- `app_name`: "my-app"

Output will show:
- Git manifest status
- ArgoCD Application status (Health/Sync)
- Ingress reachability

### 3. Destroy an Application

Use the destroy tool to remove an application.

**Tool:** destroy
**Arguments:**
- `app_name`: "my-app"

This will remove manifests from Git, triggering ArgoCD to prune the resources.

### 4. Update an Application

Use the update tool to update an application based on later resources.

**Tool:** update
**Arguments:**
- `app_name`: "my-app"

This will trigger a restert on Deployment resource which will cause it to pull the latest image.

## E2E Testing

You can run the end-to-end test if you have the environment set up:

```bash
export KUBECONFIG=~/.kube/config
export GITHUB_TOKEN=...
export GITHUB_URL=...
export E2E_TEST=true
go test -v .
```
