#!/bin/bash -ex

/usr/local/bin/install-proxy-datadog.sh \
	--proxyKind nginx \
	--appId "${RUM_APP_ID}" \
	--site "${DD_SITE}" \
	--clientToken "${RUM_CLIENT_TOKEN}" \
	--sessionSampleRate 100 \
	--sessionReplaySampleRate 100

nginx -g 'daemon off;'
