package main

import (
	"context"
	"github.com/guoyk93/grace"
	"github.com/guoyk93/grace/graceconf"
	"github.com/guoyk93/grace/gracek8s"
	"github.com/guoyk93/grace/gracemain"
	"github.com/guoyk93/grace/gracex509"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"log"
	"os"
	"time"
)

const (
	SecretEZAdmisInstall = "ezadmis-install-ca"
)

type Options struct {
	Name      string `json:"name" validate:"required"`
	Namespace string `json:"opts.Namespace"`

	Mutating       bool                                         `json:"mutating"`
	AdmissionRules []admissionregistrationv1.RuleWithOperations `json:"admissionRules" validate:"required"`
	SideEffect     admissionregistrationv1.SideEffectClass      `json:"sideEffect" default:"Unknown" validate:"required"`
	FailurePolicy  admissionregistrationv1.FailurePolicyType    `json:"failurePolicy" default:"Fail" validate:"required"`

	Image            string                        `json:"image" validate:"required"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets"`
	Affinity         *corev1.Affinity              `json:"affinity"`
	NodeSelector     map[string]string             `json:"nodeSelector"`
	ServiceAccount   string                        `json:"serviceAccount"`
	Port             int                           `json:"port" default:"443" validate:"required"`
	Env              []corev1.EnvVar               `json:"env"`
	Command          []string                      `json:"command"`
	Args             []string                      `json:"args"`
	TLSCrtPath       string                        `json:"tlsCrtPath" default:"/admission-server/tls.crt" validate:"required"`
	TLSKeyPath       string                        `json:"tlsKeyPath" default:"/admission-server/tls.key" validate:"required"`
	Volumes          []corev1.Volume               `json:"volumes"`
	VolumeMounts     []corev1.VolumeMount          `json:"volumeMounts"`
	Containers       []corev1.Container            `json:"containers"`
	Resources        corev1.ResourceRequirements   `json:"resources"`
	InitContainers   []corev1.Container            `json:"initContainers"`
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lmsgprefix)

	var err error
	defer gracemain.Exit(&err)
	defer grace.Guard(&err)

	opts := grace.Must(graceconf.LoadJSONFlagConf[Options]())

	client := grace.Must(gracek8s.DefaultClient())

	// determine namespace
	if opts.Namespace == "" {
		if opts.Namespace, err = gracek8s.InClusterNamespace(); err != nil {
			err = nil
		}
	}
	if opts.Namespace == "" {
		opts.Namespace = metav1.NamespaceDefault
	}

	log.Println("bootstrapping admission webhook", opts.Name, "in namespace", opts.Namespace)

	ctx := context.Background()

	ca := grace.Must(
		gracek8s.GetOrCreateTLSSecret(
			ctx,
			client.CoreV1().Secrets(opts.Namespace),
			SecretEZAdmisInstall,
			gracex509.GenerateOptions{
				IsCA:  true,
				Names: []string{"EZAdmisInstall root ca"},
			},
		),
	)

	log.Println("ca certificate ensured:", string(ca.Crt))

	secretName := opts.Name + "-crt"

	leaf := grace.Must(
		gracek8s.GetOrCreateTLSSecret(
			ctx,
			client.CoreV1().Secrets(opts.Namespace),
			secretName,
			gracex509.GenerateOptions{
				Parent: ca,
				Names: []string{
					opts.Name,
					opts.Name + "." + opts.Namespace,
					opts.Name + "." + opts.Namespace + ".svc",
					opts.Name + "." + opts.Namespace + ".svc.cluster",
					opts.Name + "." + opts.Namespace + ".svc.cluster.local",
				},
			},
		),
	)

	log.Println("leaf certificate ensured:", string(leaf.Crt))

	workloadSelector := map[string]string{
		"k8s-app": opts.Name,
	}

	grace.Must(gracek8s.GetOrCreate[corev1.Service](
		ctx,
		client.CoreV1().Services(opts.Namespace),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.Name,
			},
			Spec: corev1.ServiceSpec{
				Selector: workloadSelector,
				Type:     corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{
					{
						Name:       "https",
						Protocol:   corev1.ProtocolTCP,
						Port:       int32(opts.Port),
						TargetPort: intstr.FromInt(opts.Port),
					},
				},
			},
		},
	))

	log.Println("service ensured:", opts.Name)

	const volumeNameTLS = "vol-ezadmis-tls"

	grace.Must(gracek8s.GetOrCreate[appsv1.StatefulSet](
		ctx,
		client.AppsV1().StatefulSets(opts.Namespace),
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.Name,
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: workloadSelector,
				},
				ServiceName: opts.Name,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workloadSelector,
					},
					Spec: corev1.PodSpec{
						ImagePullSecrets: opts.ImagePullSecrets,
						InitContainers:   opts.InitContainers,
						Affinity:         opts.Affinity,
						NodeSelector:     opts.NodeSelector,
						Containers: append([]corev1.Container{
							{
								Name:            opts.Name,
								Image:           opts.Image,
								ImagePullPolicy: corev1.PullAlways,
								Command:         opts.Command,
								Args:            opts.Args,
								Env:             opts.Env,
								Ports: []corev1.ContainerPort{
									{
										Name:          "https",
										Protocol:      corev1.ProtocolTCP,
										ContainerPort: int32(opts.Port),
									},
								},
								VolumeMounts: append([]corev1.VolumeMount{
									{
										Name:      volumeNameTLS,
										SubPath:   corev1.TLSCertKey,
										MountPath: opts.TLSCrtPath,
									},
									{
										Name:      volumeNameTLS,
										SubPath:   corev1.TLSPrivateKeyKey,
										MountPath: opts.TLSKeyPath,
									},
								}, opts.VolumeMounts...),
								Resources: opts.Resources,
							},
						}, opts.Containers...),
						ServiceAccountName: opts.ServiceAccount,
						Volumes: append([]corev1.Volume{
							{
								Name: volumeNameTLS,
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: secretName,
									},
								},
							},
						}, opts.Volumes...),
					},
				},
			},
		}),
	)

	log.Println("statefulset ensured:", opts.Name)

	time.Sleep(time.Second * 10)

	qualifiedName := opts.Namespace + "-" + opts.Name

	if opts.Mutating {
		grace.Must(gracek8s.GetOrCreate[admissionregistrationv1.MutatingWebhookConfiguration](
			ctx,
			client.AdmissionregistrationV1().MutatingWebhookConfigurations(),
			&admissionregistrationv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: qualifiedName,
				},
				Webhooks: []admissionregistrationv1.MutatingWebhook{
					{
						Name: qualifiedName + ".ezadmis-install.guoyk93.github.io",
						ClientConfig: admissionregistrationv1.WebhookClientConfig{
							CABundle: ca.Crt,
							Service: &admissionregistrationv1.ServiceReference{
								Namespace: opts.Namespace,
								Name:      opts.Name,
							},
						},
						Rules:                   opts.AdmissionRules,
						SideEffects:             &opts.SideEffect,
						FailurePolicy:           &opts.FailurePolicy,
						AdmissionReviewVersions: []string{"v1"},
					},
				},
			},
		))
	} else {
		grace.Must(gracek8s.GetOrCreate[admissionregistrationv1.ValidatingWebhookConfiguration](
			ctx,
			client.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
			&admissionregistrationv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: qualifiedName,
				},
				Webhooks: []admissionregistrationv1.ValidatingWebhook{
					{
						Name: qualifiedName + ".ezadmis-install.guoyk93.github.io",
						ClientConfig: admissionregistrationv1.WebhookClientConfig{
							CABundle: ca.Crt,
							Service: &admissionregistrationv1.ServiceReference{
								Namespace: opts.Namespace,
								Name:      opts.Name,
							},
						},
						Rules:                   opts.AdmissionRules,
						SideEffects:             &opts.SideEffect,
						FailurePolicy:           &opts.FailurePolicy,
						AdmissionReviewVersions: []string{"v1"},
					},
				},
			},
		))
	}

	log.Println("validating/mutating webhook ensured:", opts.Name)
}
