// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controlplane

import (
	"context"
	"testing"

	"github.com/gardener/gardener-extension-provider-gcp/pkg/internal"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/util"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/extensions/pkg/webhook/controlplane/genericmutator"
	"github.com/gardener/gardener/extensions/pkg/webhook/controlplane/test"
	mockclient "github.com/gardener/gardener/pkg/mock/controller-runtime/client"

	"github.com/coreos/go-systemd/unit"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

const (
	namespace = "test"
)

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controlplane Webhook Suite")
}

var _ = Describe("Ensurer", func() {
	var (
		ctrl *gomock.Controller

		dummyContext   = genericmutator.NewEnsurerContext(nil, nil)
		eContextK8s116 = genericmutator.NewInternalEnsurerContext(
			&extensionscontroller.Cluster{
				Shoot: &gardencorev1beta1.Shoot{
					Spec: gardencorev1beta1.ShootSpec{
						Kubernetes: gardencorev1beta1.Kubernetes{
							Version: "1.16.0",
						},
					},
				},
			},
		)
		eContextK8s117 = genericmutator.NewInternalEnsurerContext(
			&extensionscontroller.Cluster{
				Shoot: &gardencorev1beta1.Shoot{
					Spec: gardencorev1beta1.ShootSpec{
						Kubernetes: gardencorev1beta1.Kubernetes{
							Version: "1.17.0",
						},
					},
				},
			},
		)

		secretKey = client.ObjectKey{Namespace: namespace, Name: v1beta1constants.SecretNameCloudProvider}
		secret    = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.SecretNameCloudProvider},
			Data:       map[string][]byte{"foo": []byte("bar")},
		}

		cmKey = client.ObjectKey{Namespace: namespace, Name: internal.CloudProviderConfigName}
		cm    = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: internal.CloudProviderConfigName},
			Data:       map[string]string{"abc": "xyz"},
		}

		annotations = map[string]string{
			"checksum/secret-" + v1beta1constants.SecretNameCloudProvider: "8bafb35ff1ac60275d62e1cbd495aceb511fb354f74a20f7d06ecb48b3a68432",
			"checksum/configmap-" + internal.CloudProviderConfigName:      "08a7bc7fe8f59b055f173145e211760a83f02cf89635cef26ebb351378635606",
		}

		kubeControllerManagerLabels = map[string]string{
			v1beta1constants.LabelNetworkPolicyToPublicNetworks:  v1beta1constants.LabelNetworkPolicyAllowed,
			v1beta1constants.LabelNetworkPolicyToPrivateNetworks: v1beta1constants.LabelNetworkPolicyAllowed,
			v1beta1constants.LabelNetworkPolicyToBlockedCIDRs:    v1beta1constants.LabelNetworkPolicyAllowed,
		}
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("#EnsureKubeAPIServerDeployment", func() {
		It("should add missing elements to kube-apiserver deployment (k8s < 1.17)", func() {
			var (
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.DeploymentNameKubeAPIServer},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "kube-apiserver",
									},
								},
							},
						},
					},
				}
			)

			// Create mock client
			client := mockclient.NewMockClient(ctrl)
			client.EXPECT().Get(context.TODO(), secretKey, &corev1.Secret{}).DoAndReturn(clientGet(secret))
			client.EXPECT().Get(context.TODO(), cmKey, &corev1.ConfigMap{}).DoAndReturn(clientGet(cm))

			// Create ensurer
			ensurer := NewEnsurer(logger)
			err := ensurer.(inject.Client).InjectClient(client)
			Expect(err).To(Not(HaveOccurred()))

			// Call EnsureKubeAPIServerDeployment method and check the result
			err = ensurer.EnsureKubeAPIServerDeployment(context.TODO(), eContextK8s116, dep, nil)
			Expect(err).To(Not(HaveOccurred()))
			checkKubeAPIServerDeployment(dep, annotations, true)
		})

		It("should add missing elements to kube-apiserver deployment (k8s >= 1.17)", func() {
			var (
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.DeploymentNameKubeAPIServer},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "kube-apiserver",
									},
								},
							},
						},
					},
				}
			)

			// Create mock client
			client := mockclient.NewMockClient(ctrl)
			client.EXPECT().Get(context.TODO(), secretKey, &corev1.Secret{}).DoAndReturn(clientGet(secret))
			client.EXPECT().Get(context.TODO(), cmKey, &corev1.ConfigMap{}).DoAndReturn(clientGet(cm))

			// Create ensurer
			ensurer := NewEnsurer(logger)
			err := ensurer.(inject.Client).InjectClient(client)
			Expect(err).To(Not(HaveOccurred()))

			// Call EnsureKubeAPIServerDeployment method and check the result
			err = ensurer.EnsureKubeAPIServerDeployment(context.TODO(), eContextK8s117, dep, nil)
			Expect(err).To(Not(HaveOccurred()))
			checkKubeAPIServerDeployment(dep, annotations, false)
		})

		It("should modify existing elements of kube-apiserver deployment", func() {
			var (
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.DeploymentNameKubeAPIServer},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "kube-apiserver",
										Command: []string{
											"--cloud-provider=?",
											"--cloud-config=?",
											"--enable-admission-plugins=Priority,NamespaceLifecycle",
											"--disable-admission-plugins=PersistentVolumeLabel",
										},
										Env: []corev1.EnvVar{
											{Name: "GOOGLE_APPLICATION_CREDENTIALS", Value: "?"},
										},
										VolumeMounts: []corev1.VolumeMount{
											{Name: internal.CloudProviderConfigName, MountPath: "?"},
											{Name: v1beta1constants.SecretNameCloudProvider, MountPath: "?"},
										},
									},
								},
								Volumes: []corev1.Volume{
									{Name: internal.CloudProviderConfigName},
									{Name: v1beta1constants.SecretNameCloudProvider},
								},
							},
						},
					},
				}
			)

			// Create mock client
			client := mockclient.NewMockClient(ctrl)
			client.EXPECT().Get(context.TODO(), secretKey, &corev1.Secret{}).DoAndReturn(clientGet(secret))
			client.EXPECT().Get(context.TODO(), cmKey, &corev1.ConfigMap{}).DoAndReturn(clientGet(cm))

			// Create ensurer
			ensurer := NewEnsurer(logger)
			err := ensurer.(inject.Client).InjectClient(client)
			Expect(err).To(Not(HaveOccurred()))

			// Call EnsureKubeAPIServerDeployment method and check the result
			err = ensurer.EnsureKubeAPIServerDeployment(context.TODO(), eContextK8s116, dep, nil)
			Expect(err).To(Not(HaveOccurred()))
			checkKubeAPIServerDeployment(dep, annotations, true)
		})
	})

	Describe("#EnsureKubeControllerManagerDeployment", func() {
		It("should add missing elements to kube-controller-manager deployment (k8s < 1.17)", func() {
			var (
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.DeploymentNameKubeControllerManager},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "kube-controller-manager",
									},
								},
							},
						},
					},
				}
			)

			// Create mock client
			client := mockclient.NewMockClient(ctrl)
			client.EXPECT().Get(context.TODO(), secretKey, &corev1.Secret{}).DoAndReturn(clientGet(secret))
			client.EXPECT().Get(context.TODO(), cmKey, &corev1.ConfigMap{}).DoAndReturn(clientGet(cm))

			// Create ensurer
			ensurer := NewEnsurer(logger)
			err := ensurer.(inject.Client).InjectClient(client)
			Expect(err).To(Not(HaveOccurred()))

			// Call EnsureKubeControllerManagerDeployment method and check the result
			err = ensurer.EnsureKubeControllerManagerDeployment(context.TODO(), eContextK8s116, dep, nil)
			Expect(err).To(Not(HaveOccurred()))
			checkKubeControllerManagerDeployment(dep, annotations, kubeControllerManagerLabels, true)
		})

		It("should add missing elements to kube-controller-manager deployment (k8s >= 1.17)", func() {
			var (
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.DeploymentNameKubeControllerManager},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "kube-controller-manager",
									},
								},
							},
						},
					},
				}
			)

			// Create mock client
			client := mockclient.NewMockClient(ctrl)
			client.EXPECT().Get(context.TODO(), secretKey, &corev1.Secret{}).DoAndReturn(clientGet(secret))
			client.EXPECT().Get(context.TODO(), cmKey, &corev1.ConfigMap{}).DoAndReturn(clientGet(cm))

			// Create ensurer
			ensurer := NewEnsurer(logger)
			err := ensurer.(inject.Client).InjectClient(client)
			Expect(err).To(Not(HaveOccurred()))

			// Call EnsureKubeControllerManagerDeployment method and check the result
			err = ensurer.EnsureKubeControllerManagerDeployment(context.TODO(), eContextK8s117, dep, nil)
			Expect(err).To(Not(HaveOccurred()))
			checkKubeControllerManagerDeployment(dep, annotations, kubeControllerManagerLabels, false)
		})

		It("should modify existing elements of kube-controller-manager deployment", func() {
			var (
				dep = &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: v1beta1constants.DeploymentNameKubeControllerManager},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name: "kube-controller-manager",
										Command: []string{
											"--cloud-provider=?",
											"--cloud-config=?",
											"--external-cloud-volume-plugin=?",
										},
										Env: []corev1.EnvVar{
											{Name: "GOOGLE_APPLICATION_CREDENTIALS", Value: "?"},
										},
										VolumeMounts: []corev1.VolumeMount{
											{Name: internal.CloudProviderConfigName, MountPath: "?"},
											{Name: v1beta1constants.SecretNameCloudProvider, MountPath: "?"},
										},
									},
								},
								Volumes: []corev1.Volume{
									{Name: internal.CloudProviderConfigName},
									{Name: v1beta1constants.SecretNameCloudProvider},
								},
							},
						},
					},
				}
			)

			// Create mock client
			client := mockclient.NewMockClient(ctrl)
			client.EXPECT().Get(context.TODO(), secretKey, &corev1.Secret{}).DoAndReturn(clientGet(secret))
			client.EXPECT().Get(context.TODO(), cmKey, &corev1.ConfigMap{}).DoAndReturn(clientGet(cm))

			// Create ensurer
			ensurer := NewEnsurer(logger)
			err := ensurer.(inject.Client).InjectClient(client)
			Expect(err).To(Not(HaveOccurred()))

			// Call EnsureKubeControllerManagerDeployment method and check the result
			err = ensurer.EnsureKubeControllerManagerDeployment(context.TODO(), eContextK8s116, dep, nil)
			Expect(err).To(Not(HaveOccurred()))
			checkKubeControllerManagerDeployment(dep, annotations, kubeControllerManagerLabels, true)
		})
	})

	Describe("#EnsureKubeletServiceUnitOptions", func() {
		It("should modify existing elements of kubelet.service unit options", func() {
			var (
				oldUnitOptions = []*unit.UnitOption{
					{
						Section: "Service",
						Name:    "ExecStart",
						Value: `/opt/bin/hyperkube kubelet \
    --config=/var/lib/kubelet/config/kubelet`,
					},
				}
				newUnitOptions = []*unit.UnitOption{
					{
						Section: "Service",
						Name:    "ExecStart",
						Value: `/opt/bin/hyperkube kubelet \
    --config=/var/lib/kubelet/config/kubelet \
    --cloud-provider=gce`,
					},
					{
						Section: "Service",
						Name:    "ExecStartPre",
						Value:   `/bin/sh -c 'hostnamectl set-hostname $(wget -q -O- --header "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/hostname | cut -d '.' -f 1)'`,
					},
				}
			)

			// Create ensurer
			ensurer := NewEnsurer(logger)

			// Call EnsureKubeletServiceUnitOptions method and check the result
			opts, err := ensurer.EnsureKubeletServiceUnitOptions(context.TODO(), dummyContext, oldUnitOptions, nil)
			Expect(err).To(Not(HaveOccurred()))
			Expect(opts).To(Equal(newUnitOptions))
		})
	})

	Describe("#EnsureKubeletConfiguration", func() {
		It("should modify existing elements of kubelet configuration", func() {
			var (
				oldKubeletConfig = &kubeletconfigv1beta1.KubeletConfiguration{
					FeatureGates: map[string]bool{
						"Foo":                      true,
						"VolumeSnapshotDataSource": true,
						"CSINodeInfo":              true,
					},
				}
				newKubeletConfig = &kubeletconfigv1beta1.KubeletConfiguration{
					FeatureGates: map[string]bool{
						"Foo": true,
					},
				}
			)

			// Create ensurer
			ensurer := NewEnsurer(logger)

			// Call EnsureKubeletConfiguration method and check the result
			kubeletConfig := *oldKubeletConfig
			err := ensurer.EnsureKubeletConfiguration(context.TODO(), dummyContext, &kubeletConfig, nil)
			Expect(err).To(Not(HaveOccurred()))
			Expect(&kubeletConfig).To(Equal(newKubeletConfig))
		})
	})

	Describe("#EnsureKubernetesGeneralConfiguration", func() {
		It("should modify existing elements of kubernetes general configuration", func() {
			var (
				modifiedData = util.StringPtr("# Default Socket Send Buffer\n" +
					"net.core.wmem_max = 16777216\n" +
					"# GCE specific settings\n" +
					"net.ipv4.ip_forward = 5\n" +
					"# For persistent HTTP connections\n" +
					"net.ipv4.tcp_slow_start_after_idle = 0")
				result = "# Default Socket Send Buffer\n" +
					"net.core.wmem_max = 16777216\n" +
					"# GCE specific settings\n" +
					"net.ipv4.ip_forward = 1\n" +
					"# For persistent HTTP connections\n" +
					"net.ipv4.tcp_slow_start_after_idle = 0"
			)
			// Create ensurer
			ensurer := NewEnsurer(logger)

			// Call EnsureKubernetesGeneralConfiguration method and check the result
			err := ensurer.EnsureKubernetesGeneralConfiguration(context.TODO(), dummyContext, modifiedData, nil)
			Expect(err).To(Not(HaveOccurred()))
			Expect(*modifiedData).To(Equal(result))
		})
		It("should add needed elements of kubernetes general configuration", func() {
			var (
				data   = util.StringPtr("# Default Socket Send Buffer\nnet.core.wmem_max = 16777216")
				result = "# Default Socket Send Buffer\n" +
					"net.core.wmem_max = 16777216\n" +
					"# GCE specific settings\n" +
					"net.ipv4.ip_forward = 1"
			)

			// Create ensurer
			ensurer := NewEnsurer(logger)

			// Call EnsureKubernetesGeneralConfiguration method and check the result
			err := ensurer.EnsureKubernetesGeneralConfiguration(context.TODO(), dummyContext, data, nil)
			Expect(err).To(Not(HaveOccurred()))
			Expect(*data).To(Equal(result))
		})
	})
})

func checkKubeAPIServerDeployment(dep *appsv1.Deployment, annotations map[string]string, k8sVersionLessThan117 bool) {
	// Check that the kube-apiserver container still exists and contains all needed command line args,
	// env vars, and volume mounts
	c := extensionswebhook.ContainerWithName(dep.Spec.Template.Spec.Containers, "kube-apiserver")
	Expect(c).To(Not(BeNil()))
	Expect(c.Command).To(ContainElement("--cloud-provider=gce"))
	Expect(c.Command).To(ContainElement("--cloud-config=/etc/kubernetes/cloudprovider/cloudprovider.conf"))
	Expect(c.Command).To(test.ContainElementWithPrefixContaining("--enable-admission-plugins=", "PersistentVolumeLabel", ","))
	Expect(c.Command).To(Not(test.ContainElementWithPrefixContaining("--disable-admission-plugins=", "PersistentVolumeLabel", ",")))
	Expect(c.Env).To(ContainElement(credentialsEnvVar))
	Expect(c.VolumeMounts).To(ContainElement(cloudProviderConfigVolumeMount))
	Expect(c.VolumeMounts).To(ContainElement(cloudProviderSecretVolumeMount))
	Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(cloudProviderConfigVolume))
	Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(cloudProviderSecretVolume))

	if !k8sVersionLessThan117 {
		Expect(c.VolumeMounts).To(ContainElement(etcSSLVolumeMount))
		Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(etcSSLVolume))
	}

	// Check that the Pod template contains all needed checksum annotations
	Expect(dep.Spec.Template.Annotations).To(Equal(annotations))
}

func checkKubeControllerManagerDeployment(dep *appsv1.Deployment, annotations, labels map[string]string, k8sVersionLessThan117 bool) {
	// Check that the kube-controller-manager container still exists and contains all needed command line args,
	// env vars, and volume mounts
	c := extensionswebhook.ContainerWithName(dep.Spec.Template.Spec.Containers, "kube-controller-manager")
	Expect(c).To(Not(BeNil()))
	Expect(c.Command).To(ContainElement("--cloud-provider=external"))
	Expect(c.Command).To(ContainElement("--cloud-config=/etc/kubernetes/cloudprovider/cloudprovider.conf"))
	Expect(c.Command).To(ContainElement("--external-cloud-volume-plugin=gce"))
	Expect(c.Env).To(ContainElement(credentialsEnvVar))
	Expect(c.VolumeMounts).To(ContainElement(cloudProviderConfigVolumeMount))
	Expect(c.VolumeMounts).To(ContainElement(cloudProviderSecretVolumeMount))
	Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(cloudProviderConfigVolume))
	Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(cloudProviderSecretVolume))

	if !k8sVersionLessThan117 {
		Expect(c.VolumeMounts).To(ContainElement(etcSSLVolumeMount))
		Expect(dep.Spec.Template.Spec.Volumes).To(ContainElement(etcSSLVolume))
	}

	// Check that the Pod template contains all needed checksum annotations
	Expect(dep.Spec.Template.Annotations).To(Equal(annotations))

	// Check that the labels for network policies are added
	Expect(dep.Spec.Template.Labels).To(Equal(labels))
}

func clientGet(result runtime.Object) interface{} {
	return func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
		switch obj.(type) {
		case *corev1.Secret:
			*obj.(*corev1.Secret) = *result.(*corev1.Secret)
		case *corev1.ConfigMap:
			*obj.(*corev1.ConfigMap) = *result.(*corev1.ConfigMap)
		}
		return nil
	}
}
