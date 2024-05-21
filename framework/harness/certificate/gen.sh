#!/bin/bash

set -e

# Generates a certificate chain for testing purposes only. Validity 10 years.
# If tests start failing because this expires, congratulations!
#
# The chain consists of a self-signed CA, followed by a CA-signed leaf certificate.
# The CA cert is generated with extra config found in ca.conf.
#
# The concatenation of the leaf + CA is loaded into the test harness's
# http server via http.ListenAndServeTLS, along with the leaf private key.
#
# At test time, the SDK-under-test will be explicitly configured to trust the CA.

leaf_cert=leaf_public.pem
leaf_private_key=leaf_private.pem

ca_cert=ca_public.pem
ca_private_key=ca_private.pem

host=localhost
certValidityDays=3650

# Create CA
openssl req -newkey rsa:4096 -keyout "${ca_private_key}" -x509 -new -nodes -out "${ca_cert}" \
  -subj "/OU=SDKTeam/O=LaunchDarkly/L=Oakland/ST=California/C=US" -days "${certValidityDays}" \
  -config ca.conf

# Create Cert Signing Request
openssl req -new -newkey rsa:4096 -nodes -keyout "${leaf_private_key}" -out csr.pem \
       -subj "/CN=${host}/OU=SDKTeam/O=LaunchDarkly/L=Oakland/ST=California/C=US"

# Sign Cert
openssl x509 -req -in csr.pem -CA "${ca_cert}" -CAkey "${ca_private_key}" -CAcreateserial -out "${leaf_cert}" \
       -days "${certValidityDays}"

rm ca_public.srl
rm csr.pem
