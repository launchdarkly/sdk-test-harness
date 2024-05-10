#!/bin/bash

set -e

# Generates a certificate for testing purposes only. Validity 10 years.
# Expires: May 7, 2034
# If tests start failing because this expires, congratulations!

openssl req -new -newkey rsa:4096 -x509 -sha256 -days 3650 -nodes -out cert.crt -keyout cert.key
