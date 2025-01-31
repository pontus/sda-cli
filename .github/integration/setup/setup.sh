#!/bin/bash

cd testing || exit 1

pip3 install s3cmd

mkdir -p keys

openssl genrsa -out dummy.ega.nbis.se.pem 4096
openssl rsa -in dummy.ega.nbis.se.pem -pubout -out keys/dummy.ega.nbis.se.pub

output=$(bash sign_jwt.sh RS256 dummy.ega.nbis.se.pem)
echo "access_token=$output" >> s3cmd.conf

# check which compose syntax to use (useful for running locally)
if ( command -v docker-compose >/dev/null 2>&1 )
then
    docker-compose up -d
else
    docker compose up -d
fi

RETRY_TIMES=0
until docker ps -f name="s3" --format "{{.Status}}" | grep "healthy"
do echo "waiting for s3 to become ready"
    RETRY_TIMES=$((RETRY_TIMES+1));
    if [ "$RETRY_TIMES" -eq 30 ]; then
        # Time out
        docker logs "s3"
        exit 1;
    fi
    sleep 10
done

RETRY_TIMES=0
until docker ps -f name="proxy" --format "{{.Status}}" | grep "Up About"
do echo "waiting for proxy to become ready"
    RETRY_TIMES=$((RETRY_TIMES+1));
    if [ "$RETRY_TIMES" -eq 30 ]; then
        # Time out
        docker logs "proxy"
        exit 1;
    fi
    sleep 10
done

RETRY_TIMES=0
until docker logs buckets | grep "Access permission for"
do echo "waiting for buckets to be created"
    RETRY_TIMES=$((RETRY_TIMES+1));
    if [ "$RETRY_TIMES" -eq 30 ]; then
        # Time out
        docker logs "buckets"
        exit 1;
    fi
    sleep 10
done

docker ps
