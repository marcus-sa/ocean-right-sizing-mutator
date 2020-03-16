#!/bin/bash
set -e
# General vars
CERTS_DIR="tls"
VERSION=$(cat VERSION)
# Output directory.
DEFAULT_OUT_DIR="$(pwd)/$CERTS_DIR"
OUT_DIR="${OUT_DIR:-$DEFAULT_OUT_DIR}"

# Namespace where webhook service and secret reside.
DEFAULT_NAMESPACE="$(kubectl config view --minify --output "jsonpath={.contexts[?(@.name=='$(kubectl config current-context)')].context.namespace}")"
NAMESPACE="${NAMESPACE:-$DEFAULT_NAMESPACE}"

# Service name of webhook.
DEFAULT_SERVICE="ocean-rs-mutator-svc"
SERVICE="${SERVICE:-$DEFAULT_SERVICE}"

# Secret name for CA certificate and server certificate/key pair.
DEFAULT_SECRET="ocean-rs-mutator-svc-secret"
SECRET="${SECRET:-$DEFAULT_SECRET}"

# Secret name for CA certificate and server certificate/key pair.
DEFAULT_VERBOSITY="1"
VERBOSITY="${VERBOSITY:-$DEFAULT_VERBOSITY}"

function generate() {
    echo "creating certs in ${OUT_DIR}"
    rm -rf "${OUT_DIR}" && mkdir -p "${OUT_DIR}"
    csr_name="${SERVICE}.${NAMESPACE}"
    
    # write the CA configuration
	cat <<EOF >>"${OUT_DIR}/ca.conf"
[ req ]
default_bits       = 2048
default_md         = sha512
default_keyfile    = ca.key
prompt             = no
encrypt_key        = yes

# base request
distinguished_name = req_distinguished_name

# extensions
req_extensions     = v3_req

# distinguished_name
[ req_distinguished_name ]
countryName            = "IL"                     # C=
organizationName       = "Spot"                   # O=
organizationalUnitName = "IT"                     # OU=
commonName             = "spot.io"                # CN=
emailAddress           = "no-reply@spot.io"       # CN/emailAddress=

# req_extensions
[ v3_req ]
# The subject alternative name extension allows various literal values to be
# included in the configuration file
# http://www.openssl.org/docs/apps/x509v3_config.html
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE}
DNS.2 = ${SERVICE}.${NAMESPACE}
DNS.3 = ${SERVICE}.${NAMESPACE}.svc
EOF
    
    # write the CA configuration
	cat <<EOF >>"${OUT_DIR}/csr.conf"
[ req ]
# base request
distinguished_name = req_distinguished_name
# extensions
req_extensions     = v3_req

# distinguished_name
[ req_distinguished_name ]

# req_extensions
[ v3_req ]
basicConstraints=CA:FALSE
subjectAltName=@alt_names
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
[alt_names]
DNS.1 = ${SERVICE}
DNS.2 = ${SERVICE}.${NAMESPACE}
DNS.3 = ${SERVICE}.${NAMESPACE}.svc
EOF
    
    
    # create new private key for the CA
    openssl genrsa \
    -out "${CERTS_DIR}/ca.key" \
    2048
    
    # create new CA cert
    openssl req \
    -new \
    -x509 \
    -key "${CERTS_DIR}/ca.key" \
    -out "${CERTS_DIR}/cacert.pem" \
    -config "${CERTS_DIR}/ca.conf"
    
    
    # create new private key for the server
    openssl genrsa \
    -out "${CERTS_DIR}/server-key.pem" \
    2048
    
    # create new CSR
    openssl req \
    -new \
    -key "${CERTS_DIR}/server-key.pem" \
    -subj "/CN=${SERVICE}.${NAMESPACE}.svc" \
    -out "${CERTS_DIR}/server.csr" \
    -config "${OUT_DIR}/csr.conf"
    
    # generate server certificate
    openssl x509 \
    -req \
    -in "${CERTS_DIR}/server.csr" \
    -CA "${CERTS_DIR}/cacert.pem" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/server-cert.pem"
    
    
    #create a secret with CA cert and server cert/key
    kubectl create secret generic "${SECRET}" \
    -n ${NAMESPACE} \
    --from-file=key.pem="${CERTS_DIR}/server-key.pem" \
    --from-file=cert.pem="${CERTS_DIR}/server-cert.pem" \
    --dry-run -o yaml > deployment/secret.yaml
    
    kubectl apply -f deployment/secret.yaml
    
    # write the CA bundle
    base64 <"${CERTS_DIR}/cacert.pem" | tr -d '\n' >"${CERTS_DIR}/ca.bundle"
    export CA_BUNDLE=$(cat $CERTS_DIR/cacert.pem | base64 | tr -d '\n')
    sed  -e s/%CA_BUNDLE%/$CA_BUNDLE/g deployment/mutatingwebhook-template.tmpl > deployment/mutatingwebhook-cabundle.yaml
    kubectl apply -f deployment/mutatingwebhook-cabundle.yaml
    
    cp deployment/service.tmpl  deployment/service.yaml
    
    kubectl apply -f deployment/service.yaml
    sed -e s/%SECRET-CERT%/$SECRET/g  -e s/%VERSION%/$VERSION/g -e s/%VERBOSITY%/$VERBOSITY/g   deployment/deployment-template.tmpl > deployment/deployment.yaml
    kubectl apply -f deployment/deployment.yaml
}

function usage() {
	cat <<EOF

Generate certificate suitable for use with webhook service. This script uses
k8s' CertificateSigningRequest API to a generate a certificate signed by k8s
CA suitable for use with webhook services. The generated server key/cert k8s
and CA cert are stored in a k8s secret.

Usage:
  ${0} [flags]

Flags:
  --service          Service name of webhook (default "${SERVICE}").
  --namespace        Namespace where webhook service and secret reside (default "${NAMESPACE}").
  --secret           Secret name for CA certificate and server certificate/key pair (default "${SECRET}").
EOF
    exit 1
}

function validate() {
    [ -z "${SERVICE}" ] && echo "ERROR: missing SERVICE name" && usage
    [ -z "${SECRET}" ] && echo "ERROR: missing SECRET name" && usage
    [ -z "${NAMESPACE}" ] && echo "ERROR: missing NAMESPACE" && usage
    
    if [ ! -x "$(command -v openssl)" ]; then
        echo "ERROR: openssl not found"
        exit 1
    fi
}

function init() {
    while [[ $# -gt 0 ]]; do
        case ${1} in
            --service)
                SERVICE="$2"
                shift
            ;;
            --secret)
                SECRET="$2"
                shift
            ;;
            --namespace)
                NAMESPACE="$2"
                shift
            ;;
            --verbosity)
                VERBOSITY="$2"
                shift
            ;;
            *)
                usage
            ;;
        esac
        shift
    done
}

function main() {
    validate
    generate
}

init "$@"
main "$@"