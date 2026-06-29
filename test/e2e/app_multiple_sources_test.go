package e2e

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/argoproj/argo-cd/v3/test/e2e/fixture"
	. "github.com/argoproj/argo-cd/v3/test/e2e/fixture/app"
	. "github.com/argoproj/argo-cd/v3/util/argo"
)

func TestMultiSourceAppCreation(t *testing.T) {
	sources := []ApplicationSource{{
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    guestbookPath,
	}, {
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    "two-nice-pods",
	}}
	ctx := Given(t)
	ctx.
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		And(func(app *Application) {
			assert.Equal(t, ctx.GetName(), app.Name)
			for i, source := range app.Spec.GetSources() {
				assert.Equal(t, sources[i].RepoURL, source.RepoURL)
				assert.Equal(t, sources[i].Path, source.Path)
			}
			assert.Equal(t, ctx.DeploymentNamespace(), app.Spec.Destination.Namespace)
			assert.Equal(t, KubernetesInternalAPIServerAddr, app.Spec.Destination.Server)
		}).
		Expect(Event(EventReasonResourceCreated, "create")).
		And(func(_ *Application) {
			// app should be listed
			output, err := RunCli("app", "list")
			require.NoError(t, err)
			assert.Contains(t, output, ctx.GetName())
		}).
		Expect(Success("")).
		Given().Timeout(60).
		When().Wait().Then().
		Expect(Success("")).
		And(func(app *Application) {
			statusByName := map[string]SyncStatusCode{}
			for _, r := range app.Status.Resources {
				statusByName[r.Name] = r.Status
			}
			// check if the app has 3 resources, guestbook and 2 pods
			assert.Len(t, statusByName, 3)
			assert.Equal(t, SyncStatusCodeSynced, statusByName["pod-1"])
			assert.Equal(t, SyncStatusCodeSynced, statusByName["pod-2"])
			assert.Equal(t, SyncStatusCodeSynced, statusByName["guestbook-ui"])
		})
}

func TestMultiSourceAppWithHelmExternalValueFiles(t *testing.T) {
	sources := []ApplicationSource{{
		RepoURL: RepoURL(RepoURLTypeFile),
		Ref:     "values",
	}, {
		RepoURL:        RepoURL(RepoURLTypeFile),
		TargetRevision: "HEAD",
		Path:           "helm-guestbook",
		Helm: &ApplicationSourceHelm{
			ReleaseName: "helm-guestbook",
			ValueFiles: []string{
				"$values/multiple-source-values/values.yaml",
			},
		},
	}}
	fmt.Printf("sources: %v\n", sources)
	ctx := Given(t)
	ctx.
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		And(func(app *Application) {
			assert.Equal(t, ctx.GetName(), app.Name)
			for i, source := range app.Spec.GetSources() {
				assert.Equal(t, sources[i].RepoURL, source.RepoURL)
				assert.Equal(t, sources[i].Path, source.Path)
			}
			assert.Equal(t, ctx.DeploymentNamespace(), app.Spec.Destination.Namespace)
			assert.Equal(t, KubernetesInternalAPIServerAddr, app.Spec.Destination.Server)
		}).
		Expect(Event(EventReasonResourceCreated, "create")).
		And(func(_ *Application) {
			// app should be listed
			output, err := RunCli("app", "list")
			require.NoError(t, err)
			assert.Contains(t, output, ctx.GetName())
		}).
		Expect(Success("")).
		Given().Timeout(60).
		When().Wait().Then().
		Expect(Success("")).
		And(func(app *Application) {
			statusByName := map[string]SyncStatusCode{}
			for _, r := range app.Status.Resources {
				statusByName[r.Name] = r.Status
			}
			assert.Len(t, statusByName, 1)
			assert.Equal(t, SyncStatusCodeSynced, statusByName["guestbook-ui"])

			// Confirm that the deployment has 3 replicas.
			output, err := Run("", "kubectl", "get", "deployment", "guestbook-ui", "-n", ctx.DeploymentNamespace(), "-o", "jsonpath={.spec.replicas}")
			require.NoError(t, err)
			assert.Equal(t, "3", output, "Expected 3 replicas for the helm-guestbook deployment")
		})
}

func TestMultiSourceAppWithSourceOverride(t *testing.T) {
	sources := []ApplicationSource{{
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    guestbookPath,
	}, {
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    "two-nice-pods",
	}, {
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    "multiple-source-values",
	}}
	ctx := Given(t)
	ctx.
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		And(func(app *Application) {
			assert.Equal(t, ctx.GetName(), app.Name)
			for i, source := range app.Spec.GetSources() {
				assert.Equal(t, sources[i].RepoURL, source.RepoURL)
				assert.Equal(t, sources[i].Path, source.Path)
			}
			assert.Equal(t, ctx.DeploymentNamespace(), app.Spec.Destination.Namespace)
			assert.Equal(t, KubernetesInternalAPIServerAddr, app.Spec.Destination.Server)
		}).
		Expect(Event(EventReasonResourceCreated, "create")).
		And(func(_ *Application) {
			// app should be listed
			output, err := RunCli("app", "list")
			require.NoError(t, err)
			assert.Contains(t, output, ctx.GetName())
		}).
		Expect(Success("")).
		Given().Timeout(60).
		When().Wait().Then().
		Expect(Success("")).
		And(func(app *Application) {
			statusByName := map[string]SyncStatusCode{}
			for _, r := range app.Status.Resources {
				statusByName[r.Name] = r.Status
			}
			// check if the app has 3 resources, guestbook and 2 pods
			assert.Len(t, statusByName, 3)
			assert.Equal(t, SyncStatusCodeSynced, statusByName["pod-1"])
			assert.Equal(t, SyncStatusCodeSynced, statusByName["pod-2"])
			assert.Equal(t, SyncStatusCodeSynced, statusByName["guestbook-ui"])

			// check if label was added to the pod to make sure resource was taken from the later source
			output, err := Run("", "kubectl", "describe", "pods", "pod-1", "-n", ctx.DeploymentNamespace())
			require.NoError(t, err)
			assert.Contains(t, output, "foo=bar")
		})
}

func TestMultiSourceAppWithSourceName(t *testing.T) {
	sources := []ApplicationSource{{
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    guestbookPath,
		Name:    "guestbook",
	}, {
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    "two-nice-pods",
		Name:    "dynamic duo",
	}}
	ctx := Given(t)
	ctx.
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		And(func(app *Application) {
			assert.Equal(t, ctx.GetName(), app.Name)
			for i, source := range app.Spec.GetSources() {
				assert.Equal(t, sources[i].RepoURL, source.RepoURL)
				assert.Equal(t, sources[i].Path, source.Path)
				assert.Equal(t, sources[i].Name, source.Name)
			}
			assert.Equal(t, ctx.DeploymentNamespace(), app.Spec.Destination.Namespace)
			assert.Equal(t, KubernetesInternalAPIServerAddr, app.Spec.Destination.Server)
		}).
		Expect(Event(EventReasonResourceCreated, "create")).
		And(func(_ *Application) {
			// we remove the first source
			output, err := RunCli("app", "remove-source", ctx.GetName(), "--source-name", sources[0].Name)
			require.NoError(t, err)
			assert.Contains(t, output, "updated successfully")
		}).
		Expect(Success("")).
		And(func(app *Application) {
			assert.Len(t, app.Spec.GetSources(), 1)
			// we add a source
			output, err := RunCli("app", "add-source", ctx.GetName(), "--source-name", sources[0].Name, "--repo", RepoURL(RepoURLTypeFile), "--path", guestbookPath)
			require.NoError(t, err)
			assert.Contains(t, output, "updated successfully")
		}).
		Expect(Success("")).
		Given().Timeout(60).
		When().Wait().Then().
		Expect(Success("")).
		And(func(app *Application) {
			assert.Len(t, app.Spec.GetSources(), 2)
			// sources order has been inverted
			assert.Equal(t, sources[1].Name, app.Spec.GetSources()[0].Name)
			assert.Equal(t, sources[0].Name, app.Spec.GetSources()[1].Name)
			statusByName := map[string]SyncStatusCode{}
			for _, r := range app.Status.Resources {
				statusByName[r.Name] = r.Status
			}
			// check if the app has 3 resources, guestbook and 2 pods
			assert.Len(t, statusByName, 3)
			assert.Equal(t, SyncStatusCodeSynced, statusByName["pod-1"])
			assert.Equal(t, SyncStatusCodeSynced, statusByName["pod-2"])
			assert.Equal(t, SyncStatusCodeSynced, statusByName["guestbook-ui"])
		})
}

func TestMultiSourceAppSetWithSourceName(t *testing.T) {
	sources := []ApplicationSource{{
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    guestbookPath,
		Name:    "guestbook",
	}, {
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    "two-nice-pods",
		Name:    "dynamic duo",
	}}
	ctx := Given(t)
	ctx.
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		And(func(app *Application) {
			assert.Equal(t, ctx.GetName(), app.Name)
			for i, source := range app.Spec.GetSources() {
				assert.Equal(t, sources[i].RepoURL, source.RepoURL)
				assert.Equal(t, sources[i].Path, source.Path)
				assert.Equal(t, sources[i].Name, source.Name)
			}
			assert.Equal(t, ctx.DeploymentNamespace(), app.Spec.Destination.Namespace)
			assert.Equal(t, KubernetesInternalAPIServerAddr, app.Spec.Destination.Server)
		}).
		Expect(Event(EventReasonResourceCreated, "create")).
		And(func(_ *Application) {
			_, err := RunCli("app", "set", ctx.GetName(), "--source-name", sources[1].Name, "--path", "deployment")
			require.NoError(t, err)
		}).
		Expect(Success("")).
		And(func(app *Application) {
			assert.Equal(t, "deployment", app.Spec.GetSources()[1].Path)
		})
}

func TestMultiSourceAppErrorWhenSourceNameAndSourcePosition(t *testing.T) {
	sources := []ApplicationSource{{
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    guestbookPath,
		Name:    "guestbook",
	}, {
		RepoURL: RepoURL(RepoURLTypeFile),
		Path:    "two-nice-pods",
		Name:    "dynamic duo",
	}}
	ctx := Given(t)
	ctx.
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		Expect(Event(EventReasonResourceCreated, "create")).
		And(func(_ *Application) {
			_, err := RunCli("app", "get", ctx.GetName(), "--source-name", sources[1].Name, "--source-position", "1")
			assert.ErrorContains(t, err, "Only one of source-position and source-name can be specified.")
		}).
		And(func(_ *Application) {
			_, err := RunCli("app", "manifests", ctx.GetName(), "--revisions", "0.0.2", "--source-names", sources[0].Name, "--revisions", "0.0.2", "--source-positions", "1")
			assert.ErrorContains(t, err, "Only one of source-positions and source-names can be specified.")
		})
}

// TestMultiSourceWebhookCacheWarm verifies that when a webhook push event is received for a
// multi-source Helm app that uses a $ref source, the manifest cache is correctly moved to the new
// revision (i.e. refSourceCommitSHAs are resolved from the git-refs cache and included in the key).
func TestMultiSourceWebhookCacheWarm(t *testing.T) {
	repoURL := RepoURL(RepoURLTypeHTTPS)
	// The GitHub webhook payload uses html_url without a .git suffix.
	htmlURL := strings.TrimSuffix(repoURL, ".git")

	sources := []ApplicationSource{{
		RepoURL:        repoURL,
		TargetRevision: "HEAD",
		Ref:            "values",
	}, {
		RepoURL:        repoURL,
		TargetRevision: "HEAD",
		Path:           "helm-guestbook",
		Helm: &ApplicationSourceHelm{
			ReleaseName: "helm-guestbook",
			ValueFiles:  []string{"$values/multiple-source-values/values.yaml"},
		},
	}}

	ctx := Given(t)
	ctx.
		HTTPSInsecureRepoURLAdded(true).
		RepoURLType(RepoURLTypeHTTPS).
		Sources(sources).
		When().
		CreateMultiSourceApp().
		Then().
		Given().Timeout(60).
		When().
		Sync().
		Wait().
		Then().
		Expect(SyncStatusIs(SyncStatusCodeSynced)).
		When().
		// Capture the current HEAD SHA (this becomes shaBefore in the webhook).
		And(func() {
			beforeSHA, err := Run(TmpDir()+"/testdata.git", "git", "rev-parse", "HEAD")
			require.NoError(t, err)
			beforeSHA = strings.TrimSpace(beforeSHA)

			// Add an unrelated file so there is a new commit, but manifest paths are unchanged
			// (so the webhook triggers cache warming instead of an app refresh).
			AddFile(t, "webhook-cache-test-unrelated.txt", "cache warm test")

			afterSHA, err := Run(TmpDir()+"/testdata.git", "git", "rev-parse", "HEAD")
			require.NoError(t, err)
			afterSHA = strings.TrimSpace(afterSHA)

			require.NotEqual(t, beforeSHA, afterSHA, "expected a new commit after AddFile")

			// Build a minimal GitHub push payload. The html_url must match the app's RepoURL so
			// that the webhook handler finds the app and attempts the cache move.
			payload := fmt.Sprintf(`{
				"ref": "refs/heads/master",
				"before": %q,
				"after": %q,
				"repository": {
					"html_url": %q,
					"default_branch": "master"
				},
				"commits": [{"added": ["webhook-cache-test-unrelated.txt"], "modified": [], "removed": []}]
			}`, beforeSHA, afterSHA, htmlURL)

			scheme := "https"
			if IsPlainText() {
				scheme = "http"
			}
			webhookURL := fmt.Sprintf("%s://%s/api/webhook", scheme, GetApiServerAddress())

			httpClient := &http.Client{
				Timeout: 10 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // e2e test uses self-signed cert
				},
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, webhookURL, bytes.NewBufferString(payload))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "push")

			resp, err := httpClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "webhook endpoint returned non-200: %s", body)

			// Give the webhook handler time to process the event and attempt the cache move.
			time.Sleep(500 * time.Millisecond)

			// Check the argocd-server metrics for a successful cache store attempt.
			metricsReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:8083/metrics", http.NoBody)
			require.NoError(t, err)
			metricsResp, err := http.DefaultClient.Do(metricsReq)
			require.NoError(t, err)
			defer metricsResp.Body.Close()
			metricsBody, err := io.ReadAll(metricsResp.Body)
			require.NoError(t, err)

			// The metric must have a successful="true" sample with a count > 0.
			assert.Contains(t, string(metricsBody), `argocd_webhook_store_cache_attempts_total{`,
				"metric argocd_webhook_store_cache_attempts_total should be present")
			assert.Contains(t, string(metricsBody), `successful="true"`,
				"should have at least one successful cache store attempt")
		})
}

// sendWebhookPush is a helper used by multi-source webhook cache warm tests.
// It POSTs a minimal GitHub push event payload to the argocd-server webhook endpoint.
func sendWebhookPush(t *testing.T, htmlURL, beforeSHA, afterSHA string) {
	t.Helper()
	payload := fmt.Sprintf(`{
		"ref": "refs/heads/master",
		"before": %q,
		"after": %q,
		"repository": {
			"html_url": %q,
			"default_branch": "master"
		},
		"commits": [{"added": [], "modified": ["webhook-cache-test-unrelated.txt"], "removed": []}]
	}`, beforeSHA, afterSHA, htmlURL)

	scheme := "https"
	if IsPlainText() {
		scheme = "http"
	}
	webhookURL := fmt.Sprintf("%s://%s/api/webhook", scheme, GetApiServerAddress())

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // e2e test uses self-signed cert
		},
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, webhookURL, bytes.NewBufferString(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")

	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "webhook endpoint returned non-200: %s", body)
}
