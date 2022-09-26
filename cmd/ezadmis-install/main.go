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

	Image          string          `json:"image" validate:"required"`
	ServiceAccount string          `json:"serviceAccount"`
	Port           int             `json:"port" default:"443" validate:"required"`
	Env            []corev1.EnvVar `json:"env"`
	MountPath      struct {
		TLSCrt string `json:"tlsCrt" default:"/admission-server/tls.crt" validate:"required"`
		TLSKey string `json:"tlsKey" default:"/admission-server/tls.key" validate:"required"`
	} `json:"mountPath"`
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lmsgprefix)

	var err error
	defer gracemain.Exit(&err)
	defer grace.Guard(&err)

	opts := grace.Must(graceconf.LoadJSONFlagConf[Options]())

	client := grace.Must(gracek8s.DefaultClient())

	// try determine namespace
	if opts.Namespace == "" {
		if opts.Namespace, err = gracek8s.InClusterNamespace(); err != nil {
			err = nil
		}
	}

	log.Println("bootstrapping admission webhook", opts.Name, "in namespace", opts.Namespace)

	ctx := context.Background()

	ca := grace.Must(gracek8s.EnsureCertificate(ctx, client, gracek8s.EnsureCertificateOptions{
		Namespace: opts.Namespace,
		Name:      SecretEZAdmisInstall,
		GenerationOptions: gracex509.GenerationOptions{
			Names: []string{"EZAdmisInstall root ca"},
		},
	}))

	log.Println("ca certificate ensured:", string(ca.CrtPEM))

	secretName := opts.Name + "-crt"

	leaf := grace.Must(gracek8s.EnsureCertificate(ctx, client, gracek8s.EnsureCertificateOptions{
		Namespace: opts.Namespace,
		Name:      secretName,
		GenerationOptions: gracex509.GenerationOptions{
			CACrtPEM: ca.CrtPEM,
			CAKeyPEM: ca.KeyPEM,
			Names: []string{
				opts.Name,
				opts.Name + "." + opts.Namespace,
				opts.Name + "." + opts.Namespace + ".svc",
				opts.Name + "." + opts.Namespace + ".svc.cluster",
				opts.Name + "." + opts.Namespace + ".svc.cluster.local",
			},
		},
	}))

	log.Println("leaf certificate ensured:", string(leaf.CrtPEM))

	serviceSelector := map[string]string{
		"k8s-app": opts.Name,
	}

	grace.Must(gracek8s.Ensure[corev1.Service](
		ctx,
		client.CoreV1().Services(opts.Namespace),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.Name,
			},
			Spec: corev1.ServiceSpec{
				Selector: serviceSelector,
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
		}))

	log.Println("service ensured:", opts.Name)

	grace.Must(gracek8s.Ensure[appsv1.StatefulSet](
		ctx,
		client.AppsV1().StatefulSets(opts.Namespace),
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.Name,
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: serviceSelector,
				},
				ServiceName: opts.Name,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: serviceSelector,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            opts.Name,
								Image:           opts.Image,
								ImagePullPolicy: corev1.PullAlways,
								Env:             opts.Env,
								Ports: []corev1.ContainerPort{
									{
										Name:          "https",
										Protocol:      corev1.ProtocolTCP,
										ContainerPort: int32(opts.Port),
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "vol-tls",
										SubPath:   corev1.TLSCertKey,
										MountPath: opts.MountPath.TLSCrt,
									},
									{
										Name:      "vol-tls",
										SubPath:   corev1.TLSPrivateKeyKey,
										MountPath: opts.MountPath.TLSKey,
									},
								},
							},
						},
						ServiceAccountName: opts.ServiceAccount,
						Volumes: []corev1.Volume{
							{
								Name: "vol-tls",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: secretName,
									},
								},
							},
						},
					},
				},
			},
		}),
	)

	log.Println("statefulset ensured:", opts.Name)

	time.Sleep(time.Second * 10)

	if opts.Mutating {
		grace.Must(gracek8s.Ensure[admissionregistrationv1.MutatingWebhookConfiguration](
			ctx,
			client.AdmissionregistrationV1().MutatingWebhookConfigurations(),
			&admissionregistrationv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: opts.Name,
				},
				Webhooks: []admissionregistrationv1.MutatingWebhook{
					{
						Name: opts.Name + ".ezadmis-install.guoyk93.github.io",
						ClientConfig: admissionregistrationv1.WebhookClientConfig{
							CABundle: ca.CrtPEM,
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
		grace.Must(gracek8s.Ensure[admissionregistrationv1.ValidatingWebhookConfiguration](
			ctx,
			client.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
			&admissionregistrationv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: opts.Name,
				},
				Webhooks: []admissionregistrationv1.ValidatingWebhook{
					{
						Name: opts.Name + ".ezadmis-install.guoyk93.github.io",
						ClientConfig: admissionregistrationv1.WebhookClientConfig{
							CABundle: ca.CrtPEM,
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
