// +build e2e

/*
Copyright 2020 Kohl's Department Stores, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"os"
	"testing"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/KohlsTechnology/eunomia/pkg/apis"
	gitopsv1alpha1 "github.com/KohlsTechnology/eunomia/pkg/apis/eunomia/v1alpha1"
	"github.com/KohlsTechnology/eunomia/pkg/util"
)

func TestModesCreateReplaceDelete(t *testing.T) {
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()

	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatalf("could not get namespace: %v", err)
	}
	if err = SetupRbacInNamespace(namespace); err != nil {
		t.Error(err)
	}

	defer DumpJobsLogsOnError(t, framework.Global, namespace)
	err = framework.AddToFrameworkScheme(apis.AddToScheme, &gitopsv1alpha1.GitOpsConfigList{})
	if err != nil {
		t.Fatal(err)
	}

	eunomiaURI, found := os.LookupEnv("EUNOMIA_URI")
	if !found {
		eunomiaURI = "https://github.com/kohlstechnology/eunomia"
	}

	eunomiaRef, found := os.LookupEnv("EUNOMIA_REF")
	if !found {
		eunomiaRef = "master"
	}

	// Step 1: create initial CR with "Create" mode, check that pods are started

	gitops := &gitopsv1alpha1.GitOpsConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GitOpsConfig",
			APIVersion: "eunomia.kohls.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gitops-modes",
			Namespace: namespace,
		},
		Spec: gitopsv1alpha1.GitOpsConfigSpec{
			TemplateSource: gitopsv1alpha1.GitConfig{
				URI:        eunomiaURI,
				Ref:        eunomiaRef,
				ContextDir: "test/e2e/testdata/modes/template1",
			},
			ParameterSource: gitopsv1alpha1.GitConfig{
				URI:        eunomiaURI,
				Ref:        eunomiaRef,
				ContextDir: "test/e2e/testdata/empty-yaml",
			},
			Triggers: []gitopsv1alpha1.GitOpsTrigger{
				{Type: "Change"},
			},
			TemplateProcessorImage: "quay.io/kohlstechnology/eunomia-base:dev",
			ResourceHandlingMode:   "Create",
			ResourceDeletionMode:   "Delete",
			ServiceAccountRef:      "eunomia-operator",
		},
	}
	gitops.Annotations = map[string]string{"gitopsconfig.eunomia.kohls.io/initialized": "true"}

	err = framework.Global.Client.Create(context.TODO(), gitops, &framework.CleanupOptions{TestContext: ctx, Timeout: timeout, RetryInterval: retryInterval})
	if err != nil {
		t.Fatal(err)
	}

	err = WaitForPodWithImage(t, framework.Global, namespace, "hello-world-modes", "hello-app:1.0", retryInterval, timeout)
	if err != nil {
		t.Error(err)
	}

	// Step 2: change the CR to a different version of image, using "Replace" mode, then verify pod change

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := framework.Global.Client.Get(context.TODO(), util.GetNN(gitops), gitops)
		if err != nil {
			t.Fatal(err)
		}
		gitops.Spec.TemplateSource.ContextDir = "test/e2e/testdata/modes/template2"
		gitops.Spec.ResourceHandlingMode = "Replace"
		err = framework.Global.Client.Update(context.Background(), gitops)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the main "hello-world-modes" app gets upgraded
	err = WaitForPodWithImage(t, framework.Global, namespace, "hello-world-modes", "hello-app:2.0", retryInterval, timeout)
	if err != nil {
		t.Error(err)
	}

	// Step 2: change the CR to "Delete" mode, then verify that the Pod is deleted

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := framework.Global.Client.Get(context.TODO(), util.GetNN(gitops), gitops)
		if err != nil {
			t.Fatal(err)
		}
		gitops.Spec.ResourceHandlingMode = "Delete"
		err = framework.Global.Client.Update(context.Background(), gitops)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Verify that the pod corresponding to the missing resource gets deleted
	err = WaitForPodAbsence(t, framework.Global, namespace, "hello-world-modes", "hello-app:2.0", retryInterval, timeout)
	if err != nil {
		t.Error(err)
	}
}
