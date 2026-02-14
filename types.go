package main

// Application holds the configuration for an application deployment
type Application struct {
	Name      string
	Image     string
	Namespace string
	Domain    string
}

// IngressConfig holds configuration specifically for Ingress options
type IngressConfig struct {
	Host string
}

// DeploymentConfig holds configuration for the Deployment resource
type DeploymentConfig struct {
	Name      string
	Image     string
	Namespace string
}

// ServiceConfig holds configuration for the Service resource
type ServiceConfig struct {
	Name      string
	Namespace string
}

// ArgoAppConfig holds configuration for the ArgoCD Application resource
type ArgoAppConfig struct {
	Name         string
	Namespace    string
	RepoURL      string
	ManifestPath string
}
