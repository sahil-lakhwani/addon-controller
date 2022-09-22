/*
Copyright 2022. projectsveltos.io. All rights reserved.

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

package controllers_test

import (
	"context"
	"crypto/sha256"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gdexlab/go-render/render"
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	"github.com/projectsveltos/cluster-api-feature-manager/controllers"
	"github.com/projectsveltos/cluster-api-feature-manager/controllers/chartmanager"
	"github.com/projectsveltos/cluster-api-feature-manager/pkg/scope"
)

var _ = Describe("HandlersHelm", func() {
	It("shouldInstall returns false when requested version does not match installed version", func() {
		currentRelease := &controllers.ReleaseInfo{
			Status:       release.StatusDeployed.String(),
			ChartVersion: "v2.5.0",
		}
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionInstall,
		}
		Expect(controllers.ShouldInstall(currentRelease, requestChart)).To(BeFalse())
	})

	It("shouldInstall returns false requested version matches installed version", func() {
		currentRelease := &controllers.ReleaseInfo{
			Status:       release.StatusDeployed.String(),
			ChartVersion: "v2.5.3",
		}
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionInstall,
		}
		Expect(controllers.ShouldInstall(currentRelease, requestChart)).To(BeFalse())
	})

	It("shouldInstall returns true when there is no current installed version", func() {
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionInstall,
		}
		Expect(controllers.ShouldInstall(nil, requestChart)).To(BeTrue())
	})

	It("shouldInstall returns false action is uninstall", func() {
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionUninstall,
		}
		Expect(controllers.ShouldInstall(nil, requestChart)).To(BeFalse())
	})

	It("shouldUninstall returns false when there is no current release installed", func() {
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionUninstall,
		}
		Expect(controllers.ShouldUninstall(nil, requestChart)).To(BeFalse())
	})

	It("shouldUninstall returns false when action is not Uninstall", func() {
		currentRelease := &controllers.ReleaseInfo{
			Status:       release.StatusDeployed.String(),
			ChartVersion: "v2.5.3",
		}
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionInstall,
		}
		Expect(controllers.ShouldUninstall(currentRelease, requestChart)).To(BeFalse())
	})

	It("shouldUpgrade returns true when installed release is different than requested release", func() {
		currentRelease := &controllers.ReleaseInfo{
			Status:       release.StatusDeployed.String(),
			ChartVersion: "v2.5.0",
		}
		requestChart := &configv1alpha1.HelmChart{
			ChartVersion:    "v2.5.3",
			HelmChartAction: configv1alpha1.HelmChartActionInstall,
		}
		Expect(controllers.ShouldUpgrade(currentRelease, requestChart)).To(BeTrue())
	})

	It("UpdateStatusForReferencedHelmReleases updates ClusterSummary.Status.HelmReleaseSummaries", func() {
		calicoChart := &configv1alpha1.HelmChart{
			RepositoryURL:    "https://projectcalico.docs.tigera.io/charts",
			RepositoryName:   "projectcalico",
			ChartName:        "projectcalico/tigera-operator",
			ChartVersion:     "v3.24.1",
			ReleaseName:      "calico",
			ReleaseNamespace: "calico",
			HelmChartAction:  configv1alpha1.HelmChartActionInstall,
		}

		kyvernoSummary := configv1alpha1.HelmChartSummary{
			ReleaseName:      "kyverno",
			ReleaseNamespace: "kyverno",
			Status:           configv1alpha1.HelChartStatusManaging,
		}

		clusterSummary := &configv1alpha1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: configv1alpha1.ClusterSummarySpec{
				ClusterNamespace: randomString(),
				ClusterName:      randomString(),
				ClusterFeatureSpec: configv1alpha1.ClusterFeatureSpec{
					HelmCharts: []configv1alpha1.HelmChart{*calicoChart},
				},
			},
			// List a helm chart non referenced anymore as managed
			Status: configv1alpha1.ClusterSummaryStatus{
				HelmReleaseSummaries: []configv1alpha1.HelmChartSummary{
					kyvernoSummary,
				},
			},
		}

		initObjects := []client.Object{
			clusterSummary,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		manager, err := chartmanager.GetChartManagerInstance(context.TODO(), c)
		Expect(err).To(BeNil())

		manager.RegisterClusterSummaryForCharts(clusterSummary)

		conflict, err := controllers.UpdateStatusForReferencedHelmReleases(context.TODO(), c, clusterSummary)
		Expect(err).To(BeNil())
		Expect(conflict).To(BeFalse())

		currentClusterSummary := &configv1alpha1.ClusterSummary{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: clusterSummary.Name}, currentClusterSummary)).To(Succeed())
		Expect(currentClusterSummary.Status.HelmReleaseSummaries).ToNot(BeNil())
		Expect(len(currentClusterSummary.Status.HelmReleaseSummaries)).To(Equal(2))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[0].Status).To(Equal(configv1alpha1.HelChartStatusManaging))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[0].ReleaseName).To(Equal(calicoChart.ReleaseName))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[0].ReleaseNamespace).To(Equal(calicoChart.ReleaseNamespace))

		// UpdateStatusForReferencedHelmReleases adds status for referenced releases and does not remove any
		// existing entry for non existing releases.
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[1].Status).To(Equal(kyvernoSummary.Status))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[1].ReleaseName).To(Equal(kyvernoSummary.ReleaseName))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[1].ReleaseNamespace).To(Equal(kyvernoSummary.ReleaseNamespace))
	})

	It("UpdateStatusForNonReferencedHelmReleases updates ClusterSummary.Status.HelmReleaseSummaries", func() {
		contourChart := &configv1alpha1.HelmChart{
			RepositoryURL:    "https://charts.bitnami.com/bitnami",
			RepositoryName:   "bitnami/contour",
			ChartName:        "bitnami/contour",
			ChartVersion:     "9.1.2",
			ReleaseName:      "contour-latest",
			ReleaseNamespace: "contour",
			HelmChartAction:  configv1alpha1.HelmChartActionInstall,
		}

		kyvernoSummary := configv1alpha1.HelmChartSummary{
			ReleaseName:      "kyverno",
			ReleaseNamespace: "kyverno",
			Status:           configv1alpha1.HelChartStatusManaging,
		}

		clusterSummary := &configv1alpha1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: configv1alpha1.ClusterSummarySpec{
				ClusterNamespace: randomString(),
				ClusterName:      randomString(),
				ClusterFeatureSpec: configv1alpha1.ClusterFeatureSpec{
					HelmCharts: []configv1alpha1.HelmChart{*contourChart},
				},
			},
			// List a helm chart non referenced anymore as managed
			Status: configv1alpha1.ClusterSummaryStatus{
				HelmReleaseSummaries: []configv1alpha1.HelmChartSummary{
					kyvernoSummary,
					{
						ReleaseName:      contourChart.ReleaseName,
						ReleaseNamespace: contourChart.ReleaseNamespace,
						Status:           configv1alpha1.HelChartStatusManaging,
					},
				},
			},
		}

		initObjects := []client.Object{
			clusterSummary,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		manager, err := chartmanager.GetChartManagerInstance(context.TODO(), c)
		Expect(err).To(BeNil())

		manager.RegisterClusterSummaryForCharts(clusterSummary)

		err = controllers.UpdateStatusForNonReferencedHelmReleases(context.TODO(), c, clusterSummary)
		Expect(err).To(BeNil())

		currentClusterSummary := &configv1alpha1.ClusterSummary{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: clusterSummary.Name}, currentClusterSummary)).To(Succeed())
		Expect(currentClusterSummary.Status.HelmReleaseSummaries).ToNot(BeNil())
		Expect(len(currentClusterSummary.Status.HelmReleaseSummaries)).To(Equal(1))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[0].Status).To(Equal(configv1alpha1.HelChartStatusManaging))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[0].ReleaseName).To(Equal(contourChart.ReleaseName))
		Expect(currentClusterSummary.Status.HelmReleaseSummaries[0].ReleaseNamespace).To(Equal(contourChart.ReleaseNamespace))
	})

	It("updateChartsInClusterConfiguration updates ClusterConfiguration with deployed helm releases", func() {
		chartDeployed := []configv1alpha1.Chart{
			{
				RepoURL:      "https://charts.bitnami.com/bitnami",
				ChartName:    "bitnami/contour",
				ChartVersion: "9.1.2",
				Namespace:    "projectcontour",
			},
		}

		clusterFeatureName := randomString()

		clusterSummary := &configv1alpha1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       configv1alpha1.ClusterFeatureKind,
						Name:       clusterFeatureName,
						APIVersion: "config.projectsveltos.io/v1alpha1",
					},
				},
			},
			Spec: configv1alpha1.ClusterSummarySpec{
				ClusterNamespace: randomString(),
				ClusterName:      randomString(),
			},
		}

		clusterConfiguration := &configv1alpha1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterSummary.Spec.ClusterName,
				Namespace: clusterSummary.Spec.ClusterNamespace,
			},
			Status: configv1alpha1.ClusterConfigurationStatus{
				ClusterFeatureResources: []configv1alpha1.ClusterFeatureResource{
					{ClusterFeatureName: clusterFeatureName},
				},
			},
		}

		initObjects := []client.Object{
			clusterConfiguration,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		Expect(controllers.UpdateChartsInClusterConfiguration(context.TODO(), c, clusterSummary,
			chartDeployed, klogr.New())).To(Succeed())

		currentClusterConfiguration := &configv1alpha1.ClusterConfiguration{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterConfiguration.Namespace, Name: clusterConfiguration.Name},
			currentClusterConfiguration)).To(Succeed())

		Expect(currentClusterConfiguration.Status.ClusterFeatureResources).ToNot(BeNil())
		Expect(len(currentClusterConfiguration.Status.ClusterFeatureResources)).To(Equal(1))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].ClusterFeatureName).To(Equal(clusterFeatureName))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features).ToNot(BeNil())
		Expect(len(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features)).To(Equal(1))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features[0].FeatureID).To(Equal(configv1alpha1.FeatureHelm))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features[0].Charts).ToNot(BeNil())
		Expect(len(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features[0].Charts)).To(Equal(1))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features[0].Charts[0].RepoURL).To(Equal(chartDeployed[0].RepoURL))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features[0].Charts[0].ChartName).To(Equal(chartDeployed[0].ChartName))
		Expect(currentClusterConfiguration.Status.ClusterFeatureResources[0].Features[0].Charts[0].ChartVersion).To(Equal(chartDeployed[0].ChartVersion))
	})
})

var _ = Describe("Hash methods", func() {
	It("HelmHash returns hash considering all referenced helm charts", func() {
		kyvernoChart := configv1alpha1.HelmChart{
			RepositoryURL:    "https://kyverno.github.io/kyverno/",
			RepositoryName:   "kyverno",
			ChartName:        "kyverno/kyverno",
			ChartVersion:     "v2.5.0",
			ReleaseName:      "kyverno-latest",
			ReleaseNamespace: "kyverno",
			HelmChartAction:  configv1alpha1.HelmChartActionInstall,
		}

		nginxChart := configv1alpha1.HelmChart{
			RepositoryURL:    "https://helm.nginx.com/stable/",
			RepositoryName:   "nginx-stable",
			ChartName:        "nginx-stable/nginx-ingress",
			ChartVersion:     "0.14.0",
			ReleaseName:      "nginx-latest",
			ReleaseNamespace: "nginx",
			HelmChartAction:  configv1alpha1.HelmChartActionInstall,
		}

		namespace := "reconcile" + randomString()
		clusterSummary := &configv1alpha1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: configv1alpha1.ClusterSummarySpec{
				ClusterNamespace: namespace,
				ClusterName:      randomString(),
				ClusterFeatureSpec: configv1alpha1.ClusterFeatureSpec{
					HelmCharts: []configv1alpha1.HelmChart{
						kyvernoChart,
						nginxChart,
					},
				},
			},
		}

		initObjects := []client.Object{
			clusterSummary,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		clusterSummaryScope, err := scope.NewClusterSummaryScope(scope.ClusterSummaryScopeParams{
			Client:         c,
			Logger:         klogr.New(),
			ClusterSummary: clusterSummary,
			ControllerName: "clustersummary",
		})
		Expect(err).To(BeNil())

		config := render.AsCode(kyvernoChart)
		config += render.AsCode(nginxChart)
		h := sha256.New()
		h.Write([]byte(config))
		expectHash := h.Sum(nil)

		hash, err := controllers.HelmHash(context.TODO(), c, clusterSummaryScope, klogr.New())
		Expect(err).To(BeNil())
		Expect(reflect.DeepEqual(hash, expectHash)).To(BeTrue())
	})
})