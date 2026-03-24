package main

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func TestParseOCIHelmChartRef(t *testing.T) {
	tests := []struct {
		name     string
		chartRef string
		want     ArgoHelmSource
		wantErr  bool
	}{
		{
			name:     "basic oci ref",
			chartRef: "oci://registry-1.docker.io/bitnamicharts/nginx:15.9.0",
			want: ArgoHelmSource{
				RepoURL:        "registry-1.docker.io/bitnamicharts",
				Chart:          "nginx",
				TargetRevision: "15.9.0",
			},
		},
		{
			name:     "registry with port",
			chartRef: "oci://registry.internal:5000/platform/charts/demo:1.2.3",
			want: ArgoHelmSource{
				RepoURL:        "registry.internal:5000/platform/charts",
				Chart:          "demo",
				TargetRevision: "1.2.3",
			},
		},
		{
			name:     "missing version",
			chartRef: "oci://registry-1.docker.io/bitnamicharts/nginx",
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseOCIHelmChartRef(test.chartRef)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOCIHelmChartRef returned error: %v", err)
			}
			if *got != test.want {
				t.Fatalf("unexpected parse result: got %+v want %+v", *got, test.want)
			}
		})
	}
}

func TestApplicationTemplateWithHelmSource(t *testing.T) {
	tmpl, err := template.ParseFS(templatesFS, "templates/application-helm.yaml")
	if err != nil {
		t.Fatalf("ParseFS returned error: %v", err)
	}

	data := ArgoApplicationData{
		Name:      "demo-app",
		Namespace: "applications",
		Helm: &ArgoHelmSource{
			RepoURL:        "registry-1.docker.io/bitnamicharts",
			Chart:          "nginx",
			TargetRevision: "15.9.0",
			ReleaseName:    "demo-app",
		},
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	output := rendered.String()
	checks := []string{
		"chart: nginx",
		"repoURL: registry-1.docker.io/bitnamicharts",
		"targetRevision: 15.9.0",
		"releaseName: demo-app",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Fatalf("rendered template missing %q in:\n%s", check, output)
		}
	}

	if strings.Contains(output, "path:") {
		t.Fatalf("rendered Helm template unexpectedly contains git path in:\n%s", output)
	}
}
