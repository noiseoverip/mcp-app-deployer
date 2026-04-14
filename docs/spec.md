
# Planning prompt for mcp-app-deployer

OK, let's do some typing now.
I want to build an MCP server, which makes it safe and easy to deploy applications to my Kubernetes cluster.
The server configuration would have:
- path to kubeconfig file that gives it access to a particular namespace in my cluster.
- name of kubernetes namespace to generate application resources in.
- domain for use in ingress. All apps will be deployed as [appname].[domain]
- github url and token for storing kubernetes manifests
- [argocd-app-path] path within github url to store argocd application definitions
- [manifest-path] path within github url to store kubernetes manifest of the application
Actions that it would support:
- `deploy-image` with args: app-name, container-image
- `deploy-helmchart` with args: app-name, oci-helm-chart
- `destroy` with args: app-name
- `status` with args: app-name

## Action:`deploy-image` 

This action would generate the following Kubernetes resources:
- Deployment with a single replica, no requests/limits for now. 
	- It should have health check checking port 8080
- Service
	- It should expose service on port 80.
- Ingress wired to the service above.
- ArgoCD application manifest is pointing it to install from [github-url]/[manifest-path]/[app-name]. It should be enabled for autosync.
Resources should be generated from golang templates so that it is easy to extend later on.

It would then:
- Git Push generate ArgoCD application to [github-url]/[argocd-app-path]/[app-name].yaml
- Git Push generated manifests to [manifest-path]/[app-name]/*

## Action:`deploy-helmchart`

args:
- app-name
- oci-helm-chart

This action should:
- Accept a full OCI Helm chart reference including a version, for example `oci://registry-1.docker.io/bitnamicharts/nginx:15.9.0`
- Generate an ArgoCD application manifest that points at the OCI Helm chart and overrides ingress:
  - ingress.name: [app-name]
  - ingress.host: [app-name].[domain]
- Git Push the generated ArgoCD application to [github-url]/[argocd-app-path]/[app-name].yaml

Then:
- Wait for the expected ArgoCD application to appear in the Kubernetes cluster and ArgoCD to report it is in-sync and healthy.
- Check the host configured in ingress is publicly reachable.


## Action: destroy
args:
- app-name

This action should:
- Push a git change removing both ArgoCD application ([argocd-app-path]/[app-name].yaml) and Kubernetes manifests ([manifest-path]/[app-name]/)
- Wait for the ArgoCD application to not be present in Kubernetes cluster.

## Action: status
args: 
- app-name

This action should:
- Report status of Kubernetes manifests in github related to this file. Are file present ?
- Report the status of an expected ArgoCD application in Github. Does it exist ?
- Report the status of the ArgoCD application responsible for deployment of the app.
- Report is the hostname defined in applications ingress is reachable from the public internet.
