# MCP server for deploying to Kubernetes

This walkthrough demonstrates how to use the `app-deployer` to manage Kubernetes applications.

It supports two deployment modes:

- `deploy-image` generates Kubernetes manifests from a container image and pushes them to your GitOps repository.
- `deploy-helmchart` pushes an ArgoCD Application definition that points at an OCI Helm chart.

Both modes use the same GitOps workflow: the server writes an ArgoCD application definition into your git repository and ArgoCD picks it up and deploys it into your cluster.


## Prerequisites

- A Kubernetes cluster (and `kubeconfig`)
- A GitHub repository for GitOps
- ArgoCD installed on the cluster
- ArgoCD repository configured with required credentials to pull Helm OCI images if using `deploy-helmchart` mode.
- ArgoCD repository configured read from GitHub repository where server will push manifests and application definitions.
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

### 1. Deploy an Application From an Image

Use the `deploy-image` tool to create a new deployment.

**Tool:** deploy-image
**Arguments:**
- `app_name`: "my-app"
- `image`: "nginx:latest"

This will:
- Generate Kubernetes manifests in Git.
- Create an ArgoCD Application in Git.
- Push changes to the repository. ArgoCD should then sync the app.

### 2. Deploy an Application From an OCI Helm Chart

Use the `deploy-helmchart` tool to create an ArgoCD application that installs an OCI Helm chart.

**Tool:** deploy-helmchart
**Arguments:**
- `app_name`: "my-app"
- `chart`: "oci://registry-1.docker.io/bitnamicharts/nginx:15.9.0"

This will:
- Create an ArgoCD Application in Git that points to the OCI chart.
- Push changes to the repository. ArgoCD should then sync the app.

Notes:
- The chart argument must be a full OCI chart reference including a version tag.
- ArgoCD expects OCI repo URLs without the `oci://` prefix in the generated manifest, so the server rewrites the input accordingly.

### 3. Check Status

Use the status tool to check if the application is deployed and healthy.

**Tool:** status
**Arguments:**
- `app_name`: "my-app"

Output will show:
- Git manifest status
- ArgoCD Application status (Health/Sync)
- Ingress reachability

For Helm chart deployments, status still checks the ArgoCD application by name. It may report that generated manifests are not present in Git, because the Helm mode only writes the ArgoCD Application manifest.

### 4. Destroy an Application

Use the destroy tool to remove an application.

**Tool:** destroy
**Arguments:**
- `app_name`: "my-app"

This will remove manifests from Git, triggering ArgoCD to prune the resources.

### 5. Update an Application

Use the update tool to update an application based on later resources.

**Tool:** update
**Arguments:**
- `app_name`: "my-app"

This will trigger a restart on the Deployment resource which will cause it to pull the latest image.

`update` remains intended for applications created with `deploy-image`, where a Deployment named after `app_name` exists in the target namespace.

## E2E Testing

You can run the end-to-end test if you have the environment set up:

```bash
export KUBECONFIG=~/.kube/config
export GITHUB_TOKEN=...
export GITHUB_URL=...
export E2E_TEST=true
go test -v .
```
