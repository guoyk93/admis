package main

import (
	"context"
	"encoding/json"
	"github.com/guoyk93/ezadmis"
	admissionv1 "k8s.io/api/admission/v1"
	"log"
	"os"
)

func main() {
	s := ezadmis.NewWebhookServer(
		ezadmis.WebhookServerOptions{
			Handler: func(
				ctx context.Context,
				req *admissionv1.AdmissionRequest,
				rw ezadmis.WebhookResponseWriter,
			) (err error) {
				buf, _ := json.MarshalIndent(req, "", "  ")
				log.Println(string(buf))
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
