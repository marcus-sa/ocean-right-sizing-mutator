/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"os"

	"github.com/spotinst/spotinst-sdk-go/spotinst"
	"github.com/spotinst/spotinst-sdk-go/spotinst/client"
	"github.com/spotinst/spotinst-sdk-go/spotinst/credentials"
	"github.com/spotinst/spotinst-sdk-go/spotinst/session"

	"k8s.io/klog"
)

const (
	// EnvCredentialsToken specifies the name of the environment variable points
	// to the Spotinst Token.
	EnvCredentialsToken = "SPOTINST_TOKEN"

	// EnvCredentialsAccount specifies the name of the environment variable points
	// to the Spotinst account ID.
	EnvCredentialsAccount = "SPOTINST_ACCOUNT"

	// EnvBaseURL specifies the name of the environment variable points
	// to the Spotinst API base URL.
	EnvBaseURL = "SPOTINST_BASE_URL"

	// EnvControllerID specifies the controller id of the ocean cluster which manage the K8s clkuster this webhook is deployed on
	EnvControllerID = "SPOTINST_CONTROLLER_ID"
)

var (
	// DefaultConfig is the config that holds all objects needed for this application
	DefaultConfig Config
)

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile            string
	KeyFile             string
	ControllerClusterID string
	spotClient          *client.Client
	spotSession         *session.Session
}

func initFlags() {
	klog.InitFlags(nil)
	flag.StringVar(&DefaultConfig.CertFile, "tls-cert-file", DefaultConfig.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated "+
		"after server cert).")
	flag.StringVar(&DefaultConfig.KeyFile, "tls-private-key-file", DefaultConfig.KeyFile, ""+
		"File containing the default x509 private key matching --tls-cert-file.")
	flag.Parse()

}

func initConfig() {

	cfg := spotinst.DefaultConfig()
	spotToken := os.Getenv(EnvCredentialsToken)
	spotAccount := os.Getenv(EnvCredentialsAccount)
	spotAPIURL := os.Getenv(EnvBaseURL)
	spotConterollerID := os.Getenv(EnvControllerID)

	if spotAPIURL != "" {
		cfg.WithBaseURL(spotAPIURL)

		klog.V(1).Infof("Configured base URL %s", spotAPIURL)

	}
	if spotToken != "" || spotAccount != "" {
		cfg.WithCredentials(credentials.NewStaticCredentials(spotToken, spotAccount))
	}
	if spotConterollerID != "" {
		DefaultConfig.ControllerClusterID = spotConterollerID
		klog.V(1).Info("Configured controller ID")
	}

	DefaultConfig.spotClient = client.New(cfg)
	DefaultConfig.spotSession = session.New(cfg)
}

func configTLS(config Config) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		klog.Fatal(err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
		// ClientCAs:    caCertPool,
		// TODO: uses mutual tls after we agree on what cert the apiserver should use.
		// ClientAuth:   tls.RequireAndVerifyClientCert,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			for i, rc := range rawCerts {
				klog.Infof("[%3d] raw cert: %s", i, rc)
			}
			return nil
		},
	}
}
