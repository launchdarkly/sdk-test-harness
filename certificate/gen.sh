#!/bin/bash

set -e

openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out certificates/cert.cert -keyout certificates/cert.key
