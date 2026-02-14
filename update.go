package main

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func update(ctx context.Context, appName string) (*mcp.CallToolResult, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to build kubeconfig: %v", err)), nil
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create Kubernetes client: %v", err)), nil
	}

	// Get the current deployment
	deploy, err := clientset.AppsV1().Deployments(namespace).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get deployment %s: %v", appName, err)), nil
	}

	// Patch the pod template annotation to trigger a rolling restart
	// This is the same mechanism used by `kubectl rollout restart`
	if deploy.Spec.Template.ObjectMeta.Annotations == nil {
		deploy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	deploy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update deployment %s: %v", appName, err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully triggered rolling restart for deployment %s in namespace %s", appName, namespace)), nil
}
