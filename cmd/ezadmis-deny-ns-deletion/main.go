package main

import (
	"context"
	"encoding/json"
	"github.com/guoyk93/ezadmis"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"log"
	"os"
	"strconv"
)

const (
	KeyDeletionAllowed = "ezadmis.guoyk93.github.io/deletion-allowed"
)

func main() {
	s := ezadmis.NewWebhookServer(
		ezadmis.WebhookServerOptions{
			Handler: func(
				ctx context.Context,
				request *admissionv1.AdmissionRequest,
				rw ezadmis.WebhookResponseWriter,
			) (err error) {
				var buf []byte
				if buf, err = request.OldObject.MarshalJSON(); err != nil {
					return
				}
				var ns corev1.Namespace
				if err = json.Unmarshal(buf, &ns); err != nil {
					return
				}
				if ns.Annotations != nil {
					if ok, _ := strconv.ParseBool(ns.Annotations[KeyDeletionAllowed]); ok {
						return
					}
				}
				rw.Deny("missing annotation '" + KeyDeletionAllowed + "', deletion of namespace is denied")
				return
			},
		},
	)

	err := s.ListenAndServeGracefully()

	if err != nil {
		log.Println("exited with error:", err.Error())
		os.Exit(1)
	}

}
