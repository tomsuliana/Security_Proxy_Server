#!/bin/sh

COMMON_NAME=$1
SUBJECT="/C=CA/ST=None/L=NB/O=None/CN=$COMMON_NAME"
NUM_OF_DAYS=3650

cd https
cat v3.ext | sed s/%%DOMAIN%%/"$COMMON_NAME"/g > /tmp/__v3.ext
openssl req -new -newkey rsa:2048 -sha256 -nodes -key cert.key -config /tmp/__v3.ext -subj "$SUBJECT" -out device.csr
openssl x509 -req -in device.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days $NUM_OF_DAYS -sha256 -extfile /tmp/__v3.ext 

rm -f device.csr